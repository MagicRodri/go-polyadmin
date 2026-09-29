package fiber

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// widgetIcons maps a widget's Template() to the icon shown in its
// dashboard card badge.
var widgetIcons = map[string]string{
	"admin/widgets/metric.html":       "metric",
	"admin/widgets/progress.html":     "progress",
	"admin/widgets/table.html":        "table",
	"admin/widgets/chart.html":        "chart",
	"admin/widgets/activity.html":     "activity",
	"admin/widgets/donut.html":        "donut",
	"admin/widgets/stat.html":         "stat",
	"admin/widgets/timeline.html":     "timeline",
	"admin/widgets/tabs.html":         "tabs",
	"admin/widgets/metric_group.html": "metric",
	"admin/widgets/data_table.html":   "table",
}

type filterView struct {
	Kind, Name, Label  string
	FromParam, ToParam string
	Start, End         string
	Options            []fieldOptionData
	Placeholder        string
	Searchable         bool
}

type widgetBodyView struct {
	State, Message, EmptyText string
	Empty                     bool
	Body                      template.HTML
	// RefreshDescription re-sends the card's subtitle out of band, so a
	// description computed from the filters follows a filter change.
	RefreshDescription bool
	Key, Description   string
}

func hasDescription(widget core.Widget) bool {
	d, ok := widget.(interface{ HasDescription() bool })
	return ok && d.HasDescription()
}

type cardView struct {
	Key, Title, Size, Icon, Description string
	HasDescription                      bool
	Top                                 bool
	Lazy                                bool
	URL, Trigger, Include, BodyInclude  string
	Searchable                          bool
	SearchPlaceholder                   string
	BodyState                           widgetBodyView
}

type dashboardPage struct {
	pageBase
	Filters []filterView
	Exports []core.DashboardExport
	Cards   []cardView
}

type cellView struct {
	HTML   template.HTML
	Align  string
	Strong bool
}

type dataTableView struct {
	Key         string
	Columns     []core.Column
	ColumnCount int
	Totals      []cellView
	Rows        [][]cellView
	NextURL     string
	Footer      string
	State       string
	Message     string
}

type tileView struct {
	Label, Icon, Hint, Value string
}

func (r *Renderer) cellsFor(columns []core.Column, row map[string]any) []cellView {
	cells := make([]cellView, len(columns))
	for i, c := range columns {
		if c.Empty != "" {
			c.Empty = r.t(c.Empty)
		}
		cells[i] = cellView{HTML: dataCellHTML(c, row[c.Key]), Align: c.Align, Strong: c.Strong}
	}
	return cells
}

func dataEmpty(data any) bool {
	switch d := data.(type) {
	case map[string]any:
		empty, _ := d["Empty"].(bool)
		return empty
	case core.DataTablePage:
		return d.Empty
	}
	return false
}

// widgetData renders what a widget's template expects from its raw data:
// tiles and table cells as pre-formatted views, anything else as is.
func (r *Renderer) widgetData(widget core.Widget, data any, nextURL string) any {
	switch d := data.(type) {
	case core.DataTablePage:
		view := dataTableView{Key: core.WidgetKey(widget), Columns: d.Columns, ColumnCount: len(d.Columns), NextURL: nextURL, Footer: d.Footer}
		if len(d.Rows.Totals) > 0 {
			view.Totals = r.cellsFor(d.Columns, d.Rows.Totals)
		}
		for _, row := range d.Rows.Items {
			view.Rows = append(view.Rows, r.cellsFor(d.Columns, row))
		}
		return view
	case core.TabsData:
		panels := make([]renderedPanel, 0, len(d.Panels))
		for _, panel := range d.Panels {
			body, err := r.renderWidgetTemplate(panel.Widget, r.widgetData(panel.Widget, panel.Data, ""))
			if err != nil {
				log.Printf("polyadmin: dashboard tab %q failed to render: %v", panel.Label, err)
			}
			panels = append(panels, renderedPanel{Label: panel.Label, Body: body})
		}
		return map[string]any{"Panels": panels}
	case map[string]any:
		if tiles, ok := d["Tiles"].([]core.Tile); ok {
			views := make([]tileView, len(tiles))
			for i, tile := range tiles {
				views[i] = tileView{Label: tile.Label, Icon: tile.Icon, Hint: tile.Hint, Value: decimalText(tile.Value)}
			}
			return map[string]any{"Tiles": views}
		}
	}
	return data
}

func (r *Renderer) renderWidgetTemplate(widget core.Widget, data any) (template.HTML, error) {
	tmpl, err := r.widgetTemplate(widget.Template())
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, widget.Template(), data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// describeWidget is the card's subtitle; a panicking description is logged
// and left out, like a failing widget, rather than failing the whole page.
func describeWidget(widget core.Widget, dc core.DashboardContext) (description string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("polyadmin: dashboard widget %q could not describe itself: %v", core.WidgetKey(widget), r)
			description = ""
		}
	}()
	return core.WidgetDescription(widget, dc)
}

func widgetFailure(widget core.Widget, err error) widgetBodyView {
	var unavailable *core.WidgetUnavailable
	if errors.As(err, &unavailable) {
		return widgetBodyView{State: "unavailable", Message: unavailable.Message}
	}
	log.Printf("polyadmin: dashboard widget %q failed: %v", core.WidgetKey(widget), err)
	return widgetBodyView{State: "unavailable"}
}

func (r *Renderer) filterViews(ctx context.Context, dashboard core.Dashboard, dc core.DashboardContext) ([]filterView, error) {
	views := []filterView{}
	for _, f := range dashboard.Filters {
		switch filter := f.(type) {
		case *core.DateRangeFilter:
			value := dc.DateRange(filter.Name())
			views = append(views, filterView{Kind: filter.Kind(), Name: filter.Name(), Label: filter.Label(),
				FromParam: filter.FromParam(), ToParam: filter.ToParam(),
				Start: value.Start.Format("2006-01-02"), End: value.End.Format("2006-01-02")})
		case *core.SelectFilter:
			choices, err := filter.Choices(ctx)
			if err != nil {
				log.Printf("polyadmin: dashboard filter %q could not load its choices: %v", filter.Name(), err)
				choices = nil
			}
			current := dc.String(filter.Name())
			empty := r.t("All")
			if filter.EmptyLabel() != "" {
				empty = r.t(filter.EmptyLabel())
			}
			options := []fieldOptionData{{Value: "", Label: empty, Selected: current == ""}}
			for _, c := range choices {
				options = append(options, fieldOptionData{Value: c.Value, Label: c.Label, Selected: c.Value == current})
			}
			views = append(views, filterView{Kind: filter.Kind(), Name: filter.Name(), Label: filter.Label(), Options: options, Placeholder: empty, Searchable: filter.Searchable()})
		}
	}
	return views, nil
}

func (r *Renderer) RenderDashboard(ctx context.Context, principal *core.Principal, csrfToken string, dashboard core.Dashboard, widgets []core.Widget, dc core.DashboardContext) (string, error) {
	filters, err := r.filterViews(ctx, dashboard, dc)
	if err != nil {
		return "", err
	}
	names := dashboard.FilterNames()
	include := ""
	if len(dashboard.Filters) > 0 {
		include = "#dashboard-filters"
	}
	cards := make([]cardView, 0, len(widgets))
	for _, widget := range widgets {
		key := core.WidgetKey(widget)
		lazy := core.IsLazyWidget(widget)
		triggers := []string{}
		if lazy {
			triggers = append(triggers, "load")
		}
		for _, n := range core.WidgetReloadsOn(widget, names) {
			triggers = append(triggers, "dashboard-filter-"+n+" from:body")
		}
		icon := widgetIcons[widget.Template()]
		if icon == "" {
			icon = "metric"
		}
		card := cardView{Key: key, Title: widget.Title(), Size: widget.Size(), Icon: icon,
			Description: describeWidget(widget, dc), HasDescription: hasDescription(widget), Lazy: lazy,
			Top: core.WidgetPlacement(widget) == "top",
			URL: r.basePath + "/_widgets/" + key, Trigger: strings.Join(triggers, ", "), Include: include}
		if table, ok := widget.(*core.DataTable); ok && table.Searchable {
			card.Searchable, card.SearchPlaceholder = true, table.SearchPlaceholder
		}
		bodyIncludes := []string{}
		if include != "" {
			bodyIncludes = append(bodyIncludes, include)
		}
		if card.Searchable {
			bodyIncludes = append(bodyIncludes, "#widget-search-"+key)
		}
		card.BodyInclude = strings.Join(bodyIncludes, ", ")
		if !lazy {
			body, data, err := r.renderWidgetBodyAndData(widget, 0)
			if err != nil {
				return "", err
			}
			card.BodyState = widgetBodyView{Body: body, Empty: dataEmpty(data), EmptyText: core.WidgetEmptyText(widget)}
		}
		cards = append(cards, card)
	}
	title := dashboard.Title
	if title == "" {
		title = "Dashboard"
	}
	title = r.t(title)
	data := dashboardPage{
		pageBase: r.pageBase(principal, csrfToken, title, title, "", nil, []breadcrumb{{Label: title, Active: true}}, nil),
		Filters:  filters,
		Exports:  dashboard.VisibleExports(principal, r.admin.Authorizer),
		Cards:    cards,
	}
	var buf bytes.Buffer
	if err := r.dashboard.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r *Renderer) RenderWidgetFragment(ctx context.Context, dashboard core.Dashboard, widget core.Widget, wc core.WidgetContext) (string, error) {
	data, err := core.ResolveWidgetData(ctx, widget, wc)
	state := widgetBodyView{EmptyText: core.WidgetEmptyText(widget)}
	if err != nil {
		state = widgetFailure(widget, err)
	}
	nextURL := ""
	if page, ok := data.(core.DataTablePage); ok && err == nil && page.HasNext {
		params := dashboard.QueryParams(wc.Filters)
		if wc.Search != "" {
			params.Set("search", wc.Search)
		}
		params.Set("offset", fmt.Sprint(page.NextOffset))
		nextURL = r.basePath + "/_widgets/" + core.WidgetKey(widget) + "?" + params.Encode()
	}
	var buf bytes.Buffer
	if _, isTable := widget.(*core.DataTable); isTable && wc.Offset > 0 {
		view := dataTableView{ColumnCount: len(widget.(*core.DataTable).Columns), State: state.State, Message: state.Message}
		if err == nil {
			view = r.widgetData(widget, data, nextURL).(dataTableView)
		}
		if err := r.widgets.ExecuteTemplate(&buf, "admin/widgets/data_table_rows.html", view); err != nil {
			return "", err
		}
		return buf.String(), nil
	}
	if err == nil {
		state.Empty = dataEmpty(data)
		if !state.Empty {
			body, renderErr := r.renderWidgetTemplate(widget, r.widgetData(widget, data, nextURL))
			if renderErr != nil {
				return "", renderErr
			}
			state.Body = body
		}
	}
	if hasDescription(widget) {
		state.RefreshDescription, state.Key = true, core.WidgetKey(widget)
		state.Description = describeWidget(widget, wc.DashboardContext)
	}
	if err := r.widgetBodyTpl.ExecuteTemplate(&buf, "widget-body", state); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func handleWidgetFragment(admin *core.Admin, renderers *Renderers, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.DashboardView, nil)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		key := c.Params("key")
		var widget core.Widget
		for _, w := range admin.Dashboard.VisibleWidgets(principal, admin.Authorizer) {
			if core.WidgetKey(w) == key {
				widget = w
				break
			}
		}
		if widget == nil {
			return writeNotFound(c, admin, basePath)
		}
		wc := admin.Dashboard.WidgetContext(widget, func(k string) string { return c.Query(k) }, principal)
		html, err := renderers.For(c).RenderWidgetFragment(c.Context(), *admin.Dashboard, widget, wc)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleDashboardExport(admin *core.Admin, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.DashboardView, nil)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		export, ok := admin.Dashboard.Export(c.Params("name"))
		if !ok {
			return writeNotFound(c, admin, basePath)
		}
		if admin.Authorizer != nil && !admin.Authorizer.Can(principal, core.ExportPermission(export), export) {
			return writeForbidden(c, admin, basePath)
		}
		download, err := export.Handler(c.Context(), admin.Dashboard.Context(func(k string) string { return c.Query(k) }, principal))
		if err != nil {
			return err
		}
		if download == nil {
			return fmt.Errorf("polyadmin: dashboard export %q returned no Download", export.Name)
		}
		if err := download.Validate(); err != nil {
			return err
		}
		return sendDownload(c, *download)
	}
}
