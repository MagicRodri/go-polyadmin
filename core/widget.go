package core

import "math"

// Widget is a single dashboard tile. Each type computes its own data and
// names its template, so a custom widget needs no framework change.
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

// Stat is a headline number paired with its change against the previous
// period. Metric answers "what is it now?"; Stat also answers "which way
// is it moving?".
type Stat struct {
	baseWidget
	GetStat func() (value any, delta float64)
}

// NewStat builds a Stat from a function returning the current value and
// its signed percentage change. Up is assumed good; for an inverted metric
// such as an error rate, negate the delta and say so in the title.
func NewStat(title string, getStat func() (any, float64), opts ...WidgetOption) Stat {
	return Stat{baseWidget: newBaseWidget(title, "admin/widgets/stat.html", opts), GetStat: getStat}
}

func (s Stat) GetData() any {
	value, delta := s.GetStat()
	// The template branches on Direction, not the sign of Delta:
	// html/template cannot compare against zero without a helper. Delta is
	// reported unsigned, since the arrow carries the direction.
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

// donutColors is the qualitative palette for Donut slices, spaced around
// the wheel so six categories stay distinguishable. They name theme.html's
// --chart-* variables rather than literal shades, so a Donut follows the
// active theme and is re-tuned for dark mode. No slice lands on the
// success/warning/danger hues, so it cannot be mistaken for a status.
var donutColors = [...]string{"chart-1", "chart-2", "chart-3", "chart-4", "chart-5", "chart-6"}

// Donut is a share-of-total breakdown drawn as an SVG ring with a legend,
// built from <circle> arcs -- the same no-charting-library stance as
// Chart.
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
		// A circle of circumference 100 (r=15.9155) lets stroke-dasharray take
		// percentages directly. 25 rotates the first slice to 12 o'clock; each
		// later one is pushed by its predecessors' combined share, kept
		// unrounded for precision.
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

// TimelineEntry is one dated event. Time arrives already formatted: the
// widget never parses or localizes it, so the application controls how its
// timestamps read.
type TimelineEntry struct {
	Time        string
	Title       string
	Description string
}

// Timeline is a vertical feed of dated events drawn as a rail of dots.
// Activity's flat strings suit a short "who did what" list; Timeline is
// for entries needing a timestamp and a body of their own.
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

// Container is implemented by widgets nesting other widgets. A nested
// widget's template name is known only at runtime, which html/template
// cannot execute from inside another template, so the adapter walks
// Panels() and renders each child before the container's own template
// runs.
type Container interface {
	Widget
	Panels() []TabPanel
}

// Tabs stacks several widgets into one card, showing one at a time. It
// holds no data itself, and every panel is computed on render rather than
// on first click, so a panel backed by a slow query costs the same whether
// or not anyone opens it.
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
