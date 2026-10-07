package fiber

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

var lazyCalls []core.WidgetContext

func lazyDashboard() *core.Dashboard {
	period := core.NewDateRangeFilter("period", 30, core.WithFilterLabel("Period"))
	period.Today = func() time.Time { return time.Date(2026, 9, 29, 0, 0, 0, 0, time.Local) }
	stores := core.NewDataTable("Stores", []core.Column{
		{Key: "name", Label: "Name", Strong: true},
		{Key: "count", Label: "Count", Align: "end", Format: "share"},
	}, func(ctx context.Context, wc core.WidgetContext) (core.Rows, error) {
		lazyCalls = append(lazyCalls, wc)
		items := []map[string]any{}
		for i := wc.Offset; i < wc.Offset+wc.Limit && i < 5; i++ {
			items = append(items, map[string]any{"name": fmt.Sprintf("row %d", i), "count": map[string]any{"count": float64(i), "percentage": 10.0}})
		}
		rows := core.Rows{Items: items, Total: core.Total(5)}
		if wc.Offset == 0 {
			rows.Totals = map[string]any{"name": "All"}
		}
		return rows, nil
	}, core.WithKey("stores"), core.WithDependsOn("store"),
		core.WithDescriptionFunc(func(dc core.DashboardContext) string {
			return "since " + dc.DateRange("period").Start.Format("2006-01-02")
		}))
	stores.PageSizeValue = 2
	stores.Searchable = true
	stores.TotalLabel = "Total: {total}"
	tiles := func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) {
		return []core.Tile{{Label: "Doors", Value: 12, Icon: "door", Hint: "all doors"}}, nil
	}
	return &core.Dashboard{
		Title: "Stats",
		Filters: []core.DashboardFilter{
			period,
			core.NewSelectFilter("store", core.WithFilterLabel("Store"), core.WithEmptyLabel("All stores"),
				core.WithFilterChoices(core.Choice{Value: "7", Label: "Downtown"})),
		},
		Widgets: []core.Widget{
			core.NewMetric("Legacy", func() any { return 41 }),
			core.NewMetricGroup("Overview", tiles, core.WithKey("overview"), core.WithDependsOn()),
			stores,
			core.NewMetricGroup("Broken", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) { return nil, errors.New("boom") }, core.WithKey("broken")),
			core.NewMetricGroup("Down", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) {
				return nil, &core.WidgetUnavailable{Message: "Analytics is down."}
			}, core.WithKey("down")),
			core.NewMetricGroup("Empty", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) { return nil, nil }, core.WithKey("empty"), core.WithEmptyText("Nothing here")),
			core.NewMetricGroup("Secret", tiles, core.WithKey("secret"), core.WithPermission("secret.view")),
		},
		Exports: []core.DashboardExport{{Name: "csv", Label: "Export CSV", Handler: func(ctx context.Context, dc core.DashboardContext) (*core.Download, error) {
			return &core.Download{Filename: "stats.csv", ContentType: "text/csv", Content: []byte(dc.String("store") + "," + dc.DateRange("period").Start.Format("2006-01-02"))}, nil
		}}},
	}
}

func makeLazyApp(t *testing.T, opts ...core.Option) *fiber.App {
	t.Helper()
	lazyCalls = nil
	return newTestApp(t, core.New(append([]core.Option{core.WithDashboard(lazyDashboard())}, opts...)...))
}

func getText(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	resp := doGet(t, app, path, nil)
	return resp.StatusCode, body(t, resp)
}

func TestDashboardPageRendersFiltersAndPlaceholders(t *testing.T) {
	_, page := getText(t, makeLazyApp(t), "/admin/?period_from=2026-01-01&period_to=2026-01-31&store=7")
	for _, want := range []string{`id="dashboard-filters"`, `name="period_from"`, `value="2026-01-01"`, `data-dashboard-filter="store"`, "Downtown", ">41<", `id="widget-body-overview"`, `hx-get="/admin/_widgets/stores"`, "since 2026-01-01", `formaction="/admin/_exports/csv"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Count(page, `data-widget-state="loading"`) != 6 || len(lazyCalls) != 0 {
		t.Fatalf("loading placeholders %d, calls %d", strings.Count(page, `data-widget-state="loading"`), len(lazyCalls))
	}
}

func TestDashboardTriggersFollowDependsOn(t *testing.T) {
	_, page := getText(t, makeLazyApp(t), "/admin/")
	tag := func(id string) string { return strings.SplitN(strings.SplitN(page, `id="`+id+`"`, 2)[1], ">", 2)[0] }
	if !strings.Contains(tag("widget-body-overview"), `hx-trigger="load"`) ||
		!strings.Contains(tag("widget-body-stores"), `hx-trigger="load, dashboard-filter-store from:body"`) ||
		!strings.Contains(tag("widget-body-broken"), "dashboard-filter-period from:body, dashboard-filter-store from:body") {
		t.Fatal(page)
	}
}

func TestFragmentRendersTiles(t *testing.T) {
	_, b := getText(t, makeLazyApp(t), "/admin/_widgets/overview")
	if !strings.Contains(b, "data-tile") || !strings.Contains(b, "all doors") || !strings.Contains(b, `data-value="12"`) {
		t.Fatal(b)
	}
}

func TestFragmentFirstPageAndLaterPage(t *testing.T) {
	app := makeLazyApp(t)
	_, first := getText(t, app, "/admin/_widgets/stores?store=7&search=ac")
	if len(lazyCalls) != 1 || lazyCalls[0].String("store") != "7" || lazyCalls[0].Search != "ac" || lazyCalls[0].Limit != 2 {
		t.Fatalf("calls %+v", lazyCalls)
	}
	for _, want := range []string{"data-totals", "data-table-footer", "Total: 5", `hx-get="/admin/_widgets/stores?`, "offset=2", "search=ac", "store=7"} {
		if !strings.Contains(first, want) {
			t.Fatalf("missing %q in %s", want, first)
		}
	}
	if strings.Count(first, "data-row") != 2 {
		t.Fatal("rows")
	}
	_, later := getText(t, app, "/admin/_widgets/stores?offset=4")
	if strings.Contains(later, "<table") || strings.Contains(later, "data-totals") || strings.Count(later, "data-row") != 1 || strings.Contains(later, "data-next-page") {
		t.Fatal(later)
	}
	getText(t, app, "/admin/_widgets/stores?offset=abc")
	if lazyCalls[len(lazyCalls)-1].Offset != 0 {
		t.Fatal("garbage offset")
	}
}

func TestFragmentUnavailableAndGenericErrors(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	app := makeLazyApp(t)
	status, down := getText(t, app, "/admin/_widgets/down")
	if status != 200 || !strings.Contains(down, `data-widget-state="unavailable"`) || !strings.Contains(down, "Analytics is down.") {
		t.Fatal(down)
	}
	status, broken := getText(t, app, "/admin/_widgets/broken")
	if status != 200 || !strings.Contains(broken, `data-widget-state="unavailable"`) || strings.Contains(broken, "boom") || !strings.Contains(logs.String(), "broken") {
		t.Fatal(broken, logs.String())
	}
	_, empty := getText(t, app, "/admin/_widgets/empty")
	if !strings.Contains(empty, "Nothing here") {
		t.Fatal(empty)
	}
}

type denyNamed struct{ permission string }

func (d denyNamed) Can(p *core.Principal, permission string, resource any) bool {
	return permission != d.permission
}

func TestHiddenAndUnknownWidgetsAre404(t *testing.T) {
	app := makeLazyApp(t, core.WithAuthorizer(denyNamed{"secret.view"}))
	_, page := getText(t, app, "/admin/")
	if strings.Contains(page, `id="widget-secret"`) {
		t.Fatal("secret shown")
	}
	if s, _ := getText(t, app, "/admin/_widgets/secret"); s != 404 {
		t.Fatalf("got %d", s)
	}
	if s, _ := getText(t, app, "/admin/_widgets/nope"); s != 404 {
		t.Fatalf("got %d", s)
	}
}

func TestDashboardExport(t *testing.T) {
	app := makeLazyApp(t)
	resp := doGet(t, app, "/admin/_exports/csv?period_from=2026-01-01&period_to=2026-01-31&store=7", nil)
	if resp.StatusCode != 200 || body(t, resp) != "7,2026-01-01" || !strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="stats.csv"`) {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if s, _ := getText(t, app, "/admin/_exports/nope"); s != 404 {
		t.Fatalf("got %d", s)
	}
	denied := makeLazyApp(t, core.WithAuthorizer(denyNamed{"dashboard.export"}))
	if _, page := getText(t, denied, "/admin/"); strings.Contains(page, "data-dashboard-export") {
		t.Fatal("export button shown")
	}
	if s, _ := getText(t, denied, "/admin/_exports/csv"); s != 403 {
		t.Fatalf("got %d", s)
	}
}

func TestAFailingChoicesFuncOrDescriptionDoesNotBreakThePage(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)
	dashboard := &core.Dashboard{
		Filters: []core.DashboardFilter{core.NewSelectFilter("store", core.WithFilterChoicesFunc(func(ctx context.Context) ([]core.Choice, error) {
			return nil, errors.New("analytics down")
		}))},
		Widgets: []core.Widget{core.NewMetricGroup("Overview", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) { return nil, nil },
			core.WithKey("overview"), core.WithDescriptionFunc(func(dc core.DashboardContext) string { panic("bad description") }))},
	}
	app := newTestApp(t, core.New(core.WithDashboard(dashboard)))
	status, page := getText(t, app, "/admin/")
	if status != 200 || !strings.Contains(page, `data-dashboard-filter="store"`) || !strings.Contains(page, `id="widget-overview"`) || !strings.Contains(logs.String(), "store") {
		t.Fatalf("status %d logs %s", status, logs.String())
	}
}

func TestALegacyWidgetIsEvaluatedOncePerPage(t *testing.T) {
	calls := 0
	dashboard := &core.Dashboard{Widgets: []core.Widget{core.NewMetric("Count", func() any { calls++; return calls })}}
	app := newTestApp(t, core.New(core.WithDashboard(dashboard)))
	getText(t, app, "/admin/")
	if calls != 1 {
		t.Fatalf("evaluated %d times", calls)
	}
}

func TestAFragmentRefreshesItsCardsDescription(t *testing.T) {
	app := makeLazyApp(t)
	if _, page := getText(t, app, "/admin/"); !strings.Contains(page, `id="widget-description-stores"`) {
		t.Fatal("no description id on the card")
	}
	_, frag := getText(t, app, "/admin/_widgets/stores?period_from=2026-01-01&period_to=2026-01-31")
	if !strings.Contains(frag, `id="widget-description-stores"`) || !strings.Contains(frag, `hx-swap-oob="true"`) || !strings.Contains(frag, "since 2026-01-01") {
		t.Fatal(frag)
	}
	if _, rows := getText(t, app, "/admin/_widgets/stores?offset=2"); strings.Contains(rows, "hx-swap-oob") {
		t.Fatal("rows-only fragment carries a description")
	}
}

func TestATopWidgetRendersAboveTheFiltersWithoutAHeader(t *testing.T) {
	tiles := func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) { return nil, nil }
	dashboard := &core.Dashboard{
		Filters: []core.DashboardFilter{core.NewSelectFilter("store", core.WithFilterChoices(core.Choice{Value: "7", Label: "Downtown"}))},
		Widgets: []core.Widget{
			core.NewMetricGroup("Hidden title", tiles, core.WithKey("overview"), core.WithPlacement("top"), core.WithDependsOn()),
			core.NewMetricGroup("Grid card", tiles, core.WithKey("grid")),
		},
	}
	app := newTestApp(t, core.New(core.WithDashboard(dashboard)))
	_, page := getText(t, app, "/admin/")
	top, filters, grid := strings.Index(page, `id="widget-overview"`), strings.Index(page, `id="dashboard-filters"`), strings.Index(page, `id="widget-grid"`)
	if !(top >= 0 && top < filters && filters < grid) {
		t.Fatalf("order top=%d filters=%d grid=%d", top, filters, grid)
	}
	card := page[top:filters]
	if strings.Contains(card, "Hidden title") || strings.Contains(card, "<h2") || !strings.Contains(card, `id="widget-body-overview"`) {
		t.Fatal(card)
	}
}

func TestPlacementMustBeGridOrTop(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic")
		}
	}()
	core.NewMetric("X", nil, core.WithPlacement("side"))
}

func TestALazyTabsOfDonutsRendersThroughItsFragment(t *testing.T) {
	donut := core.NewDonutCtx("Methods", func(ctx context.Context, wc core.WidgetContext) ([]core.ChartPoint, error) {
		return []core.ChartPoint{{Label: "Key", Value: 3}, {Label: "Face", Value: 1}}, nil
	})
	dashboard := &core.Dashboard{Widgets: []core.Widget{core.NewTabs("Breakdowns", []core.TabPanel{{Label: "Methods", Widget: donut}}, core.WithKey("breakdowns"))}}
	app := newTestApp(t, core.New(core.WithDashboard(dashboard)))
	_, page := getText(t, app, "/admin/")
	if !strings.Contains(strings.SplitN(page, `id="widget-breakdowns"`, 2)[1], `data-widget-state="loading"`) {
		t.Fatal("tabs not lazy")
	}
	_, body := getText(t, app, "/admin/_widgets/breakdowns")
	for _, want := range []string{`role="tab"`, "Methods", "Key", "Face"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestASearchableSelectFilterRendersASearchBox(t *testing.T) {
	dashboard := &core.Dashboard{Filters: []core.DashboardFilter{
		core.NewSelectFilter("store", core.WithFilterChoices(core.Choice{Value: "7", Label: "Downtown"}), core.WithSearchableSelect()),
		core.NewSelectFilter("plain", core.WithFilterChoices(core.Choice{Value: "1", Label: "One"})),
	}}
	_, page := getText(t, newTestApp(t, core.New(core.WithDashboard(dashboard))), "/admin/")
	store := strings.SplitN(strings.SplitN(page, `data-dashboard-filter="store"`, 2)[1], `data-dashboard-filter="plain"`, 2)[0]
	plain := strings.SplitN(strings.SplitN(page, `data-dashboard-filter="plain"`, 2)[1], "</form>", 2)[0]
	if !strings.Contains(store, `x-ref="search"`) || !strings.Contains(store, `x-model="query"`) || !strings.Contains(store, "No matches.") {
		t.Fatal(store)
	}
	if strings.Contains(plain, `x-ref="search"`) {
		t.Fatal("plain select got a search box")
	}
}
