package core

import (
	"context"
	"time"
)

type DateRange struct {
	Start time.Time
	End   time.Time
}

type Choice struct {
	Value string
	Label string
}

// DashboardFilter is a dashboard-wide control. It parses its own query
// parameters into a typed value and never fails on user input: anything
// it cannot read falls back to its default.
type DashboardFilter interface {
	Name() string
	Label() string
	Kind() string
	Parse(get func(string) string) any
	QueryParams(value any) map[string]string
}

type filterConfig struct {
	label       string
	emptyLabel  string
	choices     []Choice
	choicesFunc func(ctx context.Context) ([]Choice, error)
	searchable  bool
}

type FilterOption func(*filterConfig)

func WithFilterLabel(label string) FilterOption { return func(c *filterConfig) { c.label = label } }
func WithEmptyLabel(label string) FilterOption  { return func(c *filterConfig) { c.emptyLabel = label } }
func WithFilterChoices(choices ...Choice) FilterOption {
	return func(c *filterConfig) { c.choices = append([]Choice{}, choices...) }
}

// WithSearchableSelect puts a search box in a select filter's open list,
// for a long list of choices.
func WithSearchableSelect() FilterOption { return func(c *filterConfig) { c.searchable = true } }

func WithFilterChoicesFunc(fn func(ctx context.Context) ([]Choice, error)) FilterOption {
	return func(c *filterConfig) { c.choicesFunc = fn }
}

func newFilterConfig(name string, opts []FilterOption) filterConfig {
	cfg := filterConfig{label: defaultLabel(name)}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

const isoDay = "2006-01-02"

type DateRangeFilter struct {
	name        string
	cfg         filterConfig
	DefaultDays int
	// Today is the end of the default range; nil means time.Now.
	Today func() time.Time
}

func NewDateRangeFilter(name string, defaultDays int, opts ...FilterOption) *DateRangeFilter {
	return &DateRangeFilter{name: name, cfg: newFilterConfig(name, opts), DefaultDays: defaultDays}
}

func (f *DateRangeFilter) Name() string      { return f.name }
func (f *DateRangeFilter) Label() string     { return f.cfg.label }
func (f *DateRangeFilter) Kind() string      { return "date_range" }
func (f *DateRangeFilter) FromParam() string { return f.name + "_from" }
func (f *DateRangeFilter) ToParam() string   { return f.name + "_to" }

func (f *DateRangeFilter) Default() DateRange {
	now := time.Now()
	if f.Today != nil {
		now = f.Today()
	}
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return DateRange{Start: end.AddDate(0, 0, -f.DefaultDays), End: end}
}

func parseDay(raw string, fallback time.Time) time.Time {
	if raw == "" {
		return fallback
	}
	t, err := time.ParseInLocation(isoDay, raw, fallback.Location())
	if err != nil {
		return fallback
	}
	return t
}

func (f *DateRangeFilter) Parse(get func(string) string) any {
	fallback := f.Default()
	start := parseDay(get(f.FromParam()), fallback.Start)
	end := parseDay(get(f.ToParam()), fallback.End)
	if start.After(end) {
		start, end = end, start
	}
	return DateRange{Start: start, End: end}
}

func (f *DateRangeFilter) QueryParams(value any) map[string]string {
	r, _ := value.(DateRange)
	return map[string]string{f.FromParam(): r.Start.Format(isoDay), f.ToParam(): r.End.Format(isoDay)}
}

type SelectFilter struct {
	name string
	cfg  filterConfig
}

func NewSelectFilter(name string, opts ...FilterOption) *SelectFilter {
	return &SelectFilter{name: name, cfg: newFilterConfig(name, opts)}
}

func (f *SelectFilter) Name() string       { return f.name }
func (f *SelectFilter) Label() string      { return f.cfg.label }
func (f *SelectFilter) Kind() string       { return "select" }
func (f *SelectFilter) EmptyLabel() string { return f.cfg.emptyLabel }
func (f *SelectFilter) Searchable() bool   { return f.cfg.searchable }

func (f *SelectFilter) Parse(get func(string) string) any { return get(f.name) }

func (f *SelectFilter) QueryParams(value any) map[string]string {
	if s, _ := value.(string); s != "" {
		return map[string]string{f.name: s}
	}
	return map[string]string{}
}

// Choices is only called to render the page: a fragment request trusts
// the posted value rather than loading every choice to check it.
func (f *SelectFilter) Choices(ctx context.Context) ([]Choice, error) {
	if f.cfg.choicesFunc != nil {
		return f.cfg.choicesFunc(ctx)
	}
	return f.cfg.choices, nil
}
