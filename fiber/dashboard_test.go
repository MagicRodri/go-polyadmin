package fiber

import (
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestNoDashboardConfiguredRedirectsToFirstResource(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 307 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestDashboardRendersAtAdminRoot(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.createUser("a@example.com", true)
	userAdmin.createUser("b@example.com", true)

	dashboard := &core.Dashboard{
		Title:   "Overview",
		Widgets: []core.Widget{core.NewMetric("Users", func() any { return len(userAdmin.store) })},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	text := body(t, resp)
	if !strings.Contains(text, "Overview") || !strings.Contains(text, ">2<") {
		t.Fatalf("got %s", text)
	}
}

func TestDashboardOmitsWidgetDeniedByAuthorizer(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{
			core.NewMetric("Users", func() any { return 1 }),
			core.NewMetric("Revenue", func() any { return 1 }, core.WithPermission("analytics.revenue.view")),
		},
	}
	authorizer := denyRevenueAuthorizer{}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard), core.WithAuthorizer(authorizer))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	text := body(t, resp)
	if !strings.Contains(text, "Users") || strings.Contains(text, "Revenue") {
		t.Fatalf("got %s", text)
	}
}

type denyRevenueAuthorizer struct{}

func (denyRevenueAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "analytics.revenue.view"
}

func TestDashboardViewRequiresDashboardPermission(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{Widgets: []core.Widget{core.NewMetric("Users", func() any { return 1 })}}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard), core.WithAuthorizer(denyDashboardAuthorizer{}))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 403 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

type denyDashboardAuthorizer struct{}

func (denyDashboardAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != core.DashboardView
}

func TestDashboardRendersNestedTabsPanels(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{core.NewTabs("Statistics", []core.TabPanel{
			{Label: "Top products", Widget: core.NewTable("Products", []string{"Name"}, func() []map[string]any {
				return []map[string]any{{"Name": "Widget Pro"}}
			})},
			{Label: "Top customers", Widget: core.NewActivity("Customers", func() []string { return []string{"a@example.com"} })},
		})},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	text := body(t, doGet(t, app, "/admin/", nil))
	for _, want := range []string{"Top products", "Top customers", "Widget Pro", "a@example.com", `role="tablist"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestDashboardStatRendersDeltaDirection(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{core.NewStat("Sales", func() (any, float64) { return "$45,385", -12.5 })},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	text := body(t, doGet(t, app, "/admin/", nil))
	// A negative delta reads in the destructive token (not a literal
	// red), so it follows whichever theme is active -- see ui.go.
	if !strings.Contains(text, "$45,385") || !strings.Contains(text, "12.5%") || !strings.Contains(text, "text-destructive") {
		t.Fatalf("got %s", text)
	}
}

// TestALongWidgetScrollsInsideItsOwnCard: the dashboard is a grid, so a
// card that grows with its content drags every card in its row to the
// same height. The body is bounded instead, and scrolls with the themed
// scrollbar.
func TestALongWidgetScrollsInsideItsOwnCard(t *testing.T) {
	rows := make([]map[string]any, 200)
	for i := range rows {
		rows[i] = map[string]any{"Email": "user" + strconv.Itoa(i) + "@example.com"}
	}
	dashboard := &core.Dashboard{Widgets: []core.Widget{
		core.NewTable("Recent", []string{"Email"}, func() []map[string]any { return rows }),
	}}
	app := newTestApp(t, core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithDashboard(dashboard)))

	widgetBody, err := uiClasses("widget", "body")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	page := body(t, doGet(t, app, "/admin/", nil))
	if !strings.Contains(page, widgetBody) {
		t.Fatal("the widget body is not the bounded scroll box")
	}
	if !strings.Contains(widgetBody, "max-h-") || !strings.Contains(widgetBody, "overflow-y-auto") {
		t.Errorf("the body is not bounded and scrollable: %q", widgetBody)
	}
	// theme.html styles the bar; a bare overflow box would show the
	// browser's own, which is what the rest of the admin avoids.
	if !strings.Contains(widgetBody, "ui-scroll-area") {
		t.Errorf("the body does not use the themed scrollbar: %q", widgetBody)
	}
}

// TestAWidgetOwnsOnlyOneScrollBox: a table widget used to bring its own
// overflow container, which inside the bounded body meant two nested
// scrollers -- and, on a wide table, two visible scrollbars.
func TestAWidgetOwnsOnlyOneScrollBox(t *testing.T) {
	dashboard := &core.Dashboard{Widgets: []core.Widget{
		core.NewTable("Recent", []string{"Email"}, func() []map[string]any {
			return []map[string]any{{"Email": "a@example.com"}}
		}),
	}}
	app := newTestApp(t, core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithDashboard(dashboard)))

	widgetBody, _ := uiClasses("widget", "body")
	page := body(t, doGet(t, app, "/admin/", nil))
	idx := strings.Index(page, widgetBody)
	if idx < 0 {
		t.Fatal("no widget body on the page")
	}
	section := page[idx:]
	section = section[:strings.Index(section, "</table>")]
	if strings.Contains(section, "overflow-x-auto") {
		t.Error("the table widget still nests its own scroller")
	}
	// Bounded as it is, the header stays put while the rows move.
	if !strings.Contains(section, "sticky top-0") {
		t.Error("the widget table's header does not stick while it scrolls")
	}
}

// TestAWidgetNeverScrollsSideways: a card is a fixed column of the
// dashboard grid. Content wider than it has to wrap, not hand the
// reader a second scrollbar.
func TestAWidgetNeverScrollsSideways(t *testing.T) {
	dashboard := &core.Dashboard{Widgets: []core.Widget{
		core.NewTable("Recent", []string{"Email"}, func() []map[string]any {
			return []map[string]any{{"Email": "a-very-long-address-that-would-not-fit@example.com"}}
		}),
	}}
	app := newTestApp(t, core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithDashboard(dashboard)))

	widgetBody, _ := uiClasses("widget", "body")
	if !strings.Contains(widgetBody, "overflow-x-hidden") {
		t.Errorf("a widget body can still scroll sideways: %q", widgetBody)
	}
	page := body(t, doGet(t, app, "/admin/", nil))
	section := page[strings.Index(page, widgetBody):]
	section = section[:strings.Index(section, "</table>")]
	rows := section[strings.Index(section, "<tbody"):]
	if strings.Contains(rows, "whitespace-nowrap") {
		t.Error("the cells still hold their line instead of wrapping")
	}
	if !strings.Contains(rows, "break-words") {
		t.Error("a long unbroken value would still push the card sideways")
	}
}
