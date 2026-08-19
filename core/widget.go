package core

import "math"

// Widget is a single dashboard tile. Each widget type
// computes its own data via GetData and names the template that
// renders it (Template), so an application can add a custom widget
// type just by implementing Widget and pointing Template at its own
// file -- no framework change required.
type Widget interface {
	Title() string
	Size() string
	Permission() string // "" means always visible
	Template() string
	GetData() any
}

type baseWidget struct {
	title      string
	size       string
	permission string
	template   string
}

func (w baseWidget) Title() string      { return w.title }
func (w baseWidget) Size() string       { return w.size }
func (w baseWidget) Permission() string { return w.permission }
func (w baseWidget) Template() string   { return w.template }

// WidgetOption configures the common baseWidget fields shared by every
// concrete widget type below.
type WidgetOption func(*baseWidget)

func WithSize(size string) WidgetOption {
	return func(w *baseWidget) { w.size = size }
}

func WithPermission(permission string) WidgetOption {
	return func(w *baseWidget) { w.permission = permission }
}

func newBaseWidget(title, template string, opts []WidgetOption) baseWidget {
	w := baseWidget{title: title, size: "md", template: template}
	for _, opt := range opts {
		opt(&w)
	}
	return w
}

// round1 trims a computed percentage to one decimal place, since
// html/template has no arithmetic of its own and a raw float64 would
// render as "12.499999999999998".
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// Metric is a single headline number, e.g. "1,204 users".
type Metric struct {
	baseWidget
	GetValue func() any
}

func NewMetric(title string, getValue func() any, opts ...WidgetOption) Metric {
	return Metric{baseWidget: newBaseWidget(title, "admin/widgets/metric.html", opts), GetValue: getValue}
}

func (m Metric) GetData() any {
	return map[string]any{"Value": m.GetValue()}
}

// Stat is a headline number paired with its change against the
// previous period, e.g. "$45,385" and "12.5% up" -- adapted from
// Flowbite's admin-dashboard "Sales this week" card. Metric answers
// "what is it now?"; Stat also answers "which way is it moving?".
type Stat struct {
	baseWidget
	GetStat func() (value any, delta float64)
}

// NewStat builds a Stat from a function returning the current value
// and its percentage change since the previous period -- signed, so
// -4.2 means "down 4.2%". The widget assumes up is good (green) and
// down is bad (red); for a metric where that's inverted, such as an
// error rate, negate the delta and say so in the title.
func NewStat(title string, getStat func() (any, float64), opts ...WidgetOption) Stat {
	return Stat{baseWidget: newBaseWidget(title, "admin/widgets/stat.html", opts), GetStat: getStat}
}

func (s Stat) GetData() any {
	value, delta := s.GetStat()
	// The template branches on Direction rather than on the sign of
	// Delta: html/template can't compare a number against zero without
	// a helper func, and naming the three cases here keeps the arrow
	// and color choice out of the markup. Delta itself is reported
	// unsigned, since the arrow already carries the direction.
	direction := "flat"
	switch {
	case delta > 0:
		direction = "up"
	case delta < 0:
		direction = "down"
	}
	return map[string]any{"Value": value, "Delta": round1(math.Abs(delta)), "Direction": direction}
}

// Progress is a value against a target, e.g. "42 / 100 tasks complete".
type Progress struct {
	baseWidget
	GetProgress func() (value, target float64)
}

func NewProgress(title string, getProgress func() (float64, float64), opts ...WidgetOption) Progress {
	return Progress{baseWidget: newBaseWidget(title, "admin/widgets/progress.html", opts), GetProgress: getProgress}
}

func (p Progress) GetData() any {
	value, target := p.GetProgress()
	percent := 0.0
	if target > 0 {
		percent = value / target * 100
		if percent > 100 {
			percent = 100
		}
	}
	return map[string]any{"Value": value, "Target": target, "Percent": int(percent + 0.5)}
}

// Table is small tabular data: columns + rows (each row keyed by column).
type Table struct {
	baseWidget
	Columns []string
	GetRows func() []map[string]any
}

func NewTable(title string, columns []string, getRows func() []map[string]any, opts ...WidgetOption) Table {
	return Table{baseWidget: newBaseWidget(title, "admin/widgets/table.html", opts), Columns: columns, GetRows: getRows}
}

func (t Table) GetData() any {
	return map[string]any{"Columns": t.Columns, "Rows": t.GetRows()}
}

// ChartPoint is one labeled value in a Chart's series.
type ChartPoint struct {
	Label string
	Value float64
}

type chartRow struct {
	Label   string
	Value   float64
	Percent int
}

// Chart renders labeled values as simple CSS bars -- no charting
// library dependency, since the framework ships with none.
type Chart struct {
	baseWidget
	GetSeries func() []ChartPoint
}

func NewChart(title string, getSeries func() []ChartPoint, opts ...WidgetOption) Chart {
	return Chart{baseWidget: newBaseWidget(title, "admin/widgets/chart.html", opts), GetSeries: getSeries}
}

func (c Chart) GetData() any {
	series := c.GetSeries()
	max := 0.0
	for _, point := range series {
		if point.Value > max {
			max = point.Value
		}
	}
	if max == 0 {
		max = 1
	}
	rows := make([]chartRow, len(series))
	for i, point := range series {
		rows[i] = chartRow{Label: point.Label, Value: point.Value, Percent: int(point.Value/max*100 + 0.5)}
	}
	return map[string]any{"Series": rows}
}

// donutSlice is one arc + legend row for a Donut widget.
type donutSlice struct {
	Label      string
	Value      float64
	Percent    float64
	Remainder  float64 // 100 - Percent, precomputed since html/template has no arithmetic
	DashOffset float64
	Color      string
}

// donutColors is the qualitative palette for Donut slices, spaced
// around the color wheel so up to 6 categories stay visually
// distinguishable at a glance.
//
// These name shadcn/ui's --chart-* CSS variables (declared in
// templates/admin/theme.html) rather than literal Tailwind shades,
// which is the distinction shadcn itself draws: chart tokens are for
// *categorical data*, separate from the UI-chrome tokens, but still
// theme-owned. So a Donut follows the active theme and gets a palette
// re-tuned for dark mode, instead of keeping colors chosen against
// white. Overriding admin/theme.html restyles the slices along with
// everything else.
//
// A slice never lands on the success/warning/danger hues that
// templates/toasts.html uses for status, so it can't be mistaken for
// one.
var donutColors = [...]string{"chart-1", "chart-2", "chart-3", "chart-4", "chart-5", "chart-6"}

// Donut is a share-of-total breakdown, e.g. "Traffic by device"
// (Desktop / Phone / Tablet), rendered as an SVG ring with a legend --
// adapted from Flowbite's admin-dashboard "Traffic by device" card.
// Built from a handful of SVG <circle> arcs (stroke-dasharray), the
// same "no charting-library dependency" stance as Chart.
type Donut struct {
	baseWidget
	GetSeries func() []ChartPoint
}

func NewDonut(title string, getSeries func() []ChartPoint, opts ...WidgetOption) Donut {
	return Donut{baseWidget: newBaseWidget(title, "admin/widgets/donut.html", opts), GetSeries: getSeries}
}

func (d Donut) GetData() any {
	series := d.GetSeries()
	total := 0.0
	for _, point := range series {
		total += point.Value
	}
	slices := make([]donutSlice, len(series))
	cumulative := 0.0
	for i, point := range series {
		percent := 0.0
		if total > 0 {
			percent = point.Value / total * 100
		}
		// The classic SVG-ring trick: a circle with circumference 100
		// (r=15.9155) lets stroke-dasharray use percentages directly.
		// 25 rotates the first slice's start point to 12 o'clock; each
		// following slice is pushed further by its predecessors'
		// combined share (kept unrounded here for precision; only the
		// displayed Percent is rounded).
		slices[i] = donutSlice{
			Label: point.Label, Value: point.Value,
			Percent: round1(percent), Remainder: round1(100 - percent),
			DashOffset: round1(25 - cumulative),
			Color:      donutColors[i%len(donutColors)],
		}
		cumulative += percent
	}
	return map[string]any{"Slices": slices, "Total": total}
}

// Activity is a recent-activity feed: a list of short text entries.
type Activity struct {
	baseWidget
	GetEntries func() []string
}

func NewActivity(title string, getEntries func() []string, opts ...WidgetOption) Activity {
	return Activity{baseWidget: newBaseWidget(title, "admin/widgets/activity.html", opts), GetEntries: getEntries}
}

func (a Activity) GetData() any {
	return map[string]any{"Entries": a.GetEntries()}
}

// TimelineEntry is one dated event in a Timeline. Time is already
// formatted for display ("April 2023", "2h ago") -- the widget never
// parses or localizes it, so an application keeps full control of how
// its timestamps read. Description may be empty.
type TimelineEntry struct {
	Time        string
	Title       string
	Description string
}

// Timeline is a vertical feed of dated events, drawn as a rail of
// dots -- adapted from Flowbite's admin-dashboard "Latest Activity"
// card. Activity's flat strings are enough for a short "who did what"
// list; Timeline is for entries that each need a timestamp and a body
// of their own.
type Timeline struct {
	baseWidget
	GetEntries func() []TimelineEntry
}

func NewTimeline(title string, getEntries func() []TimelineEntry, opts ...WidgetOption) Timeline {
	return Timeline{baseWidget: newBaseWidget(title, "admin/widgets/timeline.html", opts), GetEntries: getEntries}
}

func (t Timeline) GetData() any {
	return map[string]any{"Entries": t.GetEntries()}
}

// TabPanel is one labeled panel of a Tabs widget, wrapping any other
// widget -- including, in principle, another Tabs.
type TabPanel struct {
	Label  string
	Widget Widget
}

// Container is implemented by widgets that nest other widgets inside
// themselves. Rendering a nested widget means executing a template
// whose name is only known at runtime, which html/template can't do
// from inside another template -- so the adapter walks Panels() and
// renders each child itself, before the container's own template
// runs. A custom container widget only has to implement this
// interface to get the same treatment.
type Container interface {
	Widget
	Panels() []TabPanel
}

// Tabs stacks several widgets into one card, showing one at a time --
// adapted from Flowbite's admin-dashboard "Statistics this month"
// card, which swaps a "Top products" table for a "Top customers" one.
// Tabs holds no data of its own; every panel's widget still computes
// its own, and all of them are computed on render (not on first
// click), so a panel backed by a slow query costs the same whether or
// not anyone opens it.
type Tabs struct {
	baseWidget
	panels []TabPanel
}

func NewTabs(title string, panels []TabPanel, opts ...WidgetOption) Tabs {
	return Tabs{baseWidget: newBaseWidget(title, "admin/widgets/tabs.html", opts), panels: panels}
}

func (t Tabs) Panels() []TabPanel { return t.panels }

// GetData reports the panels as they were configured. The adapter
// replaces each panel's Widget with its rendered HTML under the same
// "Panels" key before admin/widgets/tabs.html runs -- see Container.
func (t Tabs) GetData() any {
	return map[string]any{"Panels": t.panels}
}
