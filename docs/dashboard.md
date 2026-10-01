# Dashboard

`Dashboard` renders at `GET {basePath}` — it's independent of any one
`ModelAdmin`, just a titled collection of `Widget`s.

```go
dashboard := &core.Dashboard{
	Title: "Overview",
	Widgets: []core.Widget{
		core.NewMetric("Users", func() any { return len(users.List()) }),
		core.NewDonut("Users by status", func() []core.ChartPoint {
			// ... count active/inactive, return []core.ChartPoint{{Label: "Active", Value: ...}, ...}
		}),
	},
}
admin := core.New(core.WithModelAdmins(...), core.WithDashboard(dashboard))
```

Every widget computes its own data lazily — nothing is computed until
the dashboard route actually renders, so a widget backed by a slow
query only pays that cost on page load, not at `Dashboard` construction
time.

## How tall a card gets

The dashboard is a grid, and a grid stretches every card in a row to
match the tallest one. So a card's content area is bounded rather than
free to grow: past about 320px it scrolls inside its own card, with the
same themed scrollbar the rest of the admin uses. A `Table` of two
hundred rows leaves the cards beside it exactly where they were.

It scrolls in one direction only. A card is a fixed column of the grid,
so anything wider than it is a widget that has to wrap, not a sideways
scrollbar for the reader to drag — `widgets/table.html` wraps its cells
(and breaks a long unbroken value) rather than holding them on one
line.

This applies to a custom widget's template too — it renders inside that
box and needs no overflow container of its own. A second scroller
nested in there shows a second scrollbar, and content that bleeds past
the box's edge, as a negative margin does, is clipped.

## Built-in widget types

| Widget | Shows | Constructor |
|---|---|---|
| Metric | A single headline number | `core.NewMetric(title, func() any)` |
| Stat | A headline number *and* its change vs. the previous period | `core.NewStat(title, func() (any, float64))` |
| Progress | A value against a target, as a bar | `core.NewProgress(title, func() (float64, float64))` |
| Chart | Labeled values as horizontal CSS bars | `core.NewChart(title, func() []core.ChartPoint)` |
| Donut | A share-of-total breakdown, as an SVG ring + legend | `core.NewDonut(title, func() []core.ChartPoint)` |
| Table | Small tabular data | `core.NewTable(title, columns, func() []map[string]any)` |
| Activity | A recent-activity feed (short text entries) | `core.NewActivity(title, func() []string)` |
| Timeline | Dated events on a rail — time, title, description | `core.NewTimeline(title, func() []core.TimelineEntry)` |
| Tabs | Several widgets in one card, one visible at a time | `core.NewTabs(title, []core.TabPanel{...})` |
| MetricGroup | A row of headline numbers from one data call | `core.NewMetricGroup(title, func(ctx, wc) ([]core.Tile, error))` |
| DataTable | Paged, searchable rows fetched by the host | `core.NewDataTable(title, []core.Column{...}, func(ctx, wc) (core.Rows, error))` |

`Chart` and `Donut` are deliberately dependency-free — no JS charting
library is bundled (matching the CDN-only frontend approach), so
`Chart` renders as plain CSS width-percentage bars and `Donut` as a
hand-built SVG ring (the classic `stroke-dasharray`-on-a-circumference-
100-circle technique), not a real charting library's canvas/SVG
output. `Donut` cycles through a 6-color qualitative palette
(blue/violet/teal/amber/rose/cyan, deliberately distinct from the
toast component's green/orange/red success/warning/danger colors so a
slice is never mistaken for a status indicator); past 6 series entries
it wraps and repeats.

`Stat`, `Timeline` and `Tabs` are adapted from Flowbite's
admin-dashboard layout (its "Sales this week", "Latest Activity" and
"Statistics this month" cards respectively), restyled to the same
shadcn/ui tokens as the rest of the framework, so a custom widget
inherits the active theme (and dark mode) for free.

`Stat`'s delta is the *signed* percentage change, so `-4.2` means
"down 4.2%". The widget draws up in green and down in red, matching
the success/danger colors of the toast component, with an arrow
carrying the same meaning for anyone who can't separate the two hues.
That assumes up is good — for a metric where it isn't, such as an
error rate, negate the delta and say so in the title.

## Tabs: widgets inside widgets

`Tabs` is a *container*: it holds no data of its own, and each panel
is an ordinary widget rendered exactly as it would be at the top
level.

```go
core.NewTabs("User breakdown", []core.TabPanel{
	{Label: "By status", Widget: core.NewDonut("Users by status", ...)},
	{Label: "By organization", Widget: core.NewDonut("Users by organization", ...)},
})
```

Two things worth knowing:

- **Every panel is computed and rendered on page load**, not on first
  click — switching tabs is pure client-side Alpine, with no round
  trip, so a panel backed by a slow query costs the same whether or
  not anyone opens it. Put an expensive breakdown in its own widget
  rather than a tab if you don't want to pay for it every render.
- **A panel widget's own title isn't shown** — the tab label takes its
  place, and only the container's title appears in the card header.

With Alpine still loading (or not running at all), the first panel
stays visible and the rest stay hidden, so the card degrades to its
default tab rather than to everything-at-once.

Containers are a public extension point rather than a `Tabs`-only
special case: the adapter renders the panels of anything implementing
`core.Container` (`Widget` plus `Panels() []TabPanel`) before that
widget's own template runs, because `html/template` can't execute a
template whose name is only known at runtime. Nesting is capped at 8
levels, so a container that transitively contains itself fails with a
clear error instead of exhausting the stack.

Every widget accepts `core.WithSize("lg")` to span the full grid width
instead of one column, and an optional `core.WithPermission(...)`: a
widget naming a permission is simply omitted (not shown-disabled) if
the `Authorizer` denies it for the current principal — see
[`permissions`](permissions.md).

## Filters

A dashboard can carry a filter bar. Each filter reads its own query
parameters, so a filtered dashboard is a plain URL you can reload or share.

```go
dashboard := &core.Dashboard{
	Title: "Statistics",
	Filters: []core.DashboardFilter{
		core.NewDateRangeFilter("period", 30, core.WithFilterLabel("Period")),
		core.NewSelectFilter("contract_id", core.WithFilterLabel("Client"), core.WithEmptyLabel("All clients"),
			core.WithFilterChoicesFunc(loadContracts)),
	},
	Widgets: []core.Widget{...},
}
```

| Filter | Query parameters | Value |
|---|---|---|
| `core.NewDateRangeFilter(name, defaultDays)` | `{name}_from`, `{name}_to` (ISO dates) | `wc.DateRange(name)` |
| `core.NewSelectFilter(name, ...)` | `{name}` | `wc.String(name)` ("" when none) |

`core.WithSearchableSelect()` puts a search box in a select filter's open
list that narrows the choices by label as you type, for a long list such as
clients.

A filter never fails on what it is given: an unreadable date falls back to
the default range and a reversed range is swapped. The choices function is
only called to render the page; a widget's request passes the chosen value
through without checking it against the choices.

## Widgets that depend on the request

A widget implementing `core.ContextWidget`
(`Data(ctx context.Context, wc core.WidgetContext) (any, error)`) is loaded
after the page, from `GET {basePath}/_widgets/{key}`, so a slow service
never holds up the whole dashboard. `core.WidgetContext` carries `Filters`,
`Principal`, `Search` (the widget's own search box) and `Offset`/`Limit`
(the page a `DataTable` is asking for).

The widgets built on `GetData()` — every built-in one listed above except
`MetricGroup` and `DataTable` — keep rendering with the page, as before.

Options every widget takes:

- `core.WithKey(key)` — the widget's URL slug. Defaults to a slug of the
  title (transliterated, so Cyrillic titles work); set it explicitly to keep
  URLs stable when a title changes. Keys must be unique within a dashboard.
- `core.WithDependsOn(names...)` — the filters the widget reloads on.
  Without it a widget reloads on every filter; called with no names it never
  reloads. Changing a filter re-fetches only the widgets that depend on it.
- `core.WithDescription(text)` / `core.WithDescriptionFunc(func(core.DashboardContext) string)`
  — a subtitle, e.g. the period being shown.
- `core.WithEmptyText(text)` — shown when the widget has nothing to show.
- `core.WithSize("full")` spans the whole row.
- `core.WithPlacement("top")` renders the widget above the filter bar, full
  width and without its card header -- a summary row such as the headline
  numbers.

## MetricGroup

Several numbers that one call answers:

```go
core.NewMetricGroup("Overview", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) {
	data, err := analytics.Overview(ctx)
	if err != nil {
		return nil, err
	}
	return []core.Tile{
		{Label: "Contracts", Value: data.Contracts, Icon: "file-text"},
		{Label: "Using the portal", Value: data.Active, Icon: "activity", Hint: fmt.Sprintf("dormant: %d", data.Dormant)},
	}, nil
}, core.WithKey("overview"), core.WithSize("full"), core.WithDependsOn())
```

## DataTable

Rows the host fetches a page at a time, e.g. from another service:

```go
table := core.NewDataTable("Passages", []core.Column{
	{Key: "contract", Label: "Contract", Strong: true},
	{Key: "total", Label: "Total", Align: "end", Format: "number"},
	{Key: "keypass", Label: "By key", Align: "end", Format: "share"},
}, func(ctx context.Context, wc core.WidgetContext) (core.Rows, error) {
	page, err := analytics.Passages(ctx, wc.DateRange("period"), wc.Limit, wc.Offset)
	if err != nil {
		return core.Rows{}, err
	}
	return core.Rows{Items: page.Items, Total: core.Total(page.Total), Totals: page.Overall}, nil
}, core.WithKey("passages"), core.WithSize("full"))
table.Searchable = true
table.TotalLabel = "{total} contracts"
```

- `core.Rows{Items, Total, Totals}`: `Items` are maps keyed by column key.
  `Totals`, returned with the first page, is a bold summary row pinned above
  it. With `Total` (`core.Total(n)`) the table knows when it has shown
  everything; without it, a page shorter than the page size is the last.
- Scrolling to the last row loads the next page. `PageSizeValue` defaults to
  50; 0 loads everything at once.
- `Searchable` adds a search box to the card; its text arrives as `wc.Search`.
- `TotalLabel` is a footer with `{total}` filled in.

| `Format` | Value | Renders |
|---|---|---|
| `text` (or "") | anything | as is |
| `number` | int or float | in the viewer's locale |
| `datetime` | `time.Time` or ISO string | in the viewer's locale and time zone |
| `share` | `map[string]any{"count": 12, "percentage": 34.5}` | `12 (34.5%)`; just `0` when the count is 0 |
| `percent` | a number | `87.3%` |

`Empty: "never used"` on a column shows that text as a badge in an empty
cell instead of a dash. It goes through translation like other labels. An
unknown `Format` or `Align` makes `core.New` panic.

`Tones: []core.Tone{{Min: 80, Variant: "success"}, {Min: 50, Variant: "warning"}, {Min: 0, Variant: "danger"}}`
colours a numeric cell: it renders as a badge in the colour of the highest
`Min` the value reaches, and stays plain below all of them.

### Filter-aware donuts and tabs

`core.NewDonutCtx(title, func(ctx, wc) ([]core.ChartPoint, error))` builds a
donut drawn from the request's filters; it loads from its fragment route like
a `DataTable`. A `Tabs` loads that way as soon as one of its panels does, so a
card of breakdowns can follow the filters.

## When a widget fails

Return `&core.WidgetUnavailable{Message: "The analytics service is down."}`
to show that message in the widget's card. Any other error shows "This
section is unavailable." and is logged; either way the other widgets load
normally. A request that never reaches the admin shows "Couldn't load this
section." with a Retry button.

## Exports

A dashboard export is a button on the filter bar that submits the current
filters to `GET {basePath}/_exports/{name}`:

```go
Exports: []core.DashboardExport{{
	Name:  "xlsx",
	Label: "Export to Excel",
	Handler: func(ctx context.Context, dc core.DashboardContext) (*core.Download, error) {
		workbook, err := buildWorkbook(ctx, dc.DateRange("period"))
		if err != nil {
			return nil, err
		}
		return &core.Download{Filename: "statistics.xlsx", ContentType: xlsxContentType, Content: workbook}, nil
	},
}},
```

It requires the `dashboard.export` permission unless `Permission` names
another; without it the button is hidden and the route answers 403.

## Routes

| Route | Serves |
|---|---|
| `GET {basePath}` | the page: filter bar, exports, a card per widget |
| `GET {basePath}/_widgets/{key}` | one widget's body; rows only when `offset` > 0 |
| `GET {basePath}/_exports/{name}` | an export's file |

All of them require `dashboard.view`. A widget hidden by its permission
answers 404 on its route, as an unknown key does.

## Custom widgets

You can add your own widget type without a framework change. Implement
the `Widget` interface (`Title()`, `Size()`, `Permission()`,
`Template()`, `GetData()`) for your own type, and pass
`fiberadapter.WithTemplateDirs(...)` to `Mount` with a file at
`{dir}/{your Template() value}` defining a block named after that same
value:

```go
// mywidget.go
type RecentSignupsWidget struct{ /* ... */ }

func (w RecentSignupsWidget) Template() string { return "widgets/recent-signups.html" }
// ... Title(), Size(), Permission(), GetData()
```

```gotemplate
{{/* templates/widgets/recent-signups.html */}}
{{define "widgets/recent-signups.html"}}
  <!-- your markup, using whatever GetData() returned -->
{{end}}
```

```go
fiberadapter.Mount(group, admin, "/admin", fiberadapter.WithTemplateDirs("templates"))
```

The framework's own widget templates (`Metric`, `Stat`, `Progress`,
`Chart`, `Donut`, `Table`, `Activity`, `Timeline`, `Tabs`) are checked
first, so a custom `Template()` value only needs to avoid colliding
with `admin/widgets/*.html` — see [`templates`](templates.md) for
the full resolution order shared with per-resource overrides.

A custom widget that nests other widgets only has to implement
`core.Container` on top of `Widget`; its template then receives
`{"Panels": [{Label, Body}]}`, with each `Body` the already-rendered
HTML of that panel's widget.
