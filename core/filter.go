package core

import (
	"fmt"
	"time"
)

// Filter is a list-view constraint the user can toggle.
// Apply receives the raw, still-a-string query-param value -- parsing
// it is the filter's own job, since only it knows what its values mean.
type Filter interface {
	Name() string
	Label() string
	ChoicesWithLabels() [][2]string
	Apply(objects []any, raw string, modelAdmin ModelAdmin) []any
}

type baseFilter struct {
	name  string
	label string
}

func (f baseFilter) Name() string  { return f.name }
func (f baseFilter) Label() string { return f.label }

func newBaseFilter(name, label string) baseFilter {
	if label == "" {
		label = defaultLabel(name)
	}
	return baseFilter{name: name, label: label}
}

// BooleanFilter matches a boolean field against "true"/"false".
type BooleanFilter struct {
	baseFilter
}

func NewBooleanFilter(name string, opts ...func(*string)) BooleanFilter {
	var label string
	for _, opt := range opts {
		opt(&label)
	}
	return BooleanFilter{newBaseFilter(name, label)}
}

func (f BooleanFilter) ChoicesWithLabels() [][2]string {
	return [][2]string{{"", N_("All")}, {"true", N_("Yes")}, {"false", N_("No")}}
}

func (f BooleanFilter) Apply(objects []any, raw string, modelAdmin ModelAdmin) []any {
	if raw == "" {
		return objects
	}
	field, ok := modelAdmin.Field(f.name)
	if !ok {
		return objects
	}
	want := raw == "true" || raw == "1" || raw == "on" || raw == "yes"
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		if toBool(field.GetValue(obj)) == want {
			out = append(out, obj)
		}
	}
	return out
}

func toBool(value any) bool {
	b, _ := value.(bool)
	return b
}

// ChoiceFilter matches a field's string representation against one of
// a fixed set of choices.
type ChoiceFilter struct {
	baseFilter
	Choices []string
}

func NewChoiceFilter(name string, choices []string) ChoiceFilter {
	return ChoiceFilter{baseFilter: newBaseFilter(name, ""), Choices: choices}
}

func (f ChoiceFilter) ChoicesWithLabels() [][2]string {
	pairs := make([][2]string, 0, len(f.Choices)+1)
	pairs = append(pairs, [2]string{"", N_("All")})
	for _, choice := range f.Choices {
		pairs = append(pairs, [2]string{choice, choice})
	}
	return pairs
}

func (f ChoiceFilter) Apply(objects []any, raw string, modelAdmin ModelAdmin) []any {
	if raw == "" {
		return objects
	}
	field, ok := modelAdmin.Field(f.name)
	if !ok {
		return objects
	}
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		if stringify(field.GetValue(obj)) == raw {
			out = append(out, obj)
		}
	}
	return out
}

func stringify(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}

// DateFilter narrows a list to a window around today: the presets a
// reader actually asks for ("what came in this week?"), rather than two
// date boxes to fill in. It is declared like any other filter --
// DeclaredFilters: []core.Filter{core.NewDateFilter("Founded")} -- and so
// it renders in the same filter panel and rides in the same ListRequest,
// which means exports and delete_selected narrow with it.
//
// A ListQuerier host reads the raw value and resolves it in its own
// query; DateFilterRange turns a value into the half-open window this
// applies in memory, so the two cannot drift.
type DateFilter struct {
	baseFilter
}

func NewDateFilter(name string, opts ...func(*string)) DateFilter {
	var label string
	for _, opt := range opts {
		opt(&label)
	}
	return DateFilter{newBaseFilter(name, label)}
}

// The filter's values. Stable strings, because they end up in URLs.
const (
	DateFilterToday     = "today"
	DateFilterPast7Days = "7d"
	DateFilterThisMonth = "month"
	DateFilterThisYear  = "year"
)

func (f DateFilter) ChoicesWithLabels() [][2]string {
	return [][2]string{
		{"", N_("Any date")},
		{DateFilterToday, N_("Today")},
		{DateFilterPast7Days, N_("Past 7 days")},
		{DateFilterThisMonth, N_("This month")},
		{DateFilterThisYear, N_("This year")},
	}
}

// DateFilterRange is the half-open window [from, to) a value means,
// relative to now. ok is false for "" and for anything unrecognised, so a
// crafted URL narrows nothing rather than failing.
func DateFilterRange(raw string, now time.Time) (from, to time.Time, ok bool) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch raw {
	case DateFilterToday:
		return day, day.AddDate(0, 0, 1), true
	case DateFilterPast7Days:
		// Inclusive of today, so "past 7 days" counts seven days, not eight.
		return day.AddDate(0, 0, -6), day.AddDate(0, 0, 1), true
	case DateFilterThisMonth:
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(0, 1, 0), true
	case DateFilterThisYear:
		start := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		return start, start.AddDate(1, 0, 0), true
	}
	return time.Time{}, time.Time{}, false
}

func (f DateFilter) Apply(objects []any, raw string, modelAdmin ModelAdmin) []any {
	from, to, ok := DateFilterRange(raw, time.Now())
	if !ok {
		return objects
	}
	field, fieldOK := modelAdmin.Field(f.name)
	if !fieldOK {
		return objects
	}
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		moment, isTime := asTime(field.GetValue(obj))
		if !isTime || moment.IsZero() {
			continue
		}
		// Compared as a date: a datetime's clock time must not decide
		// whether it falls in "today".
		at := time.Date(moment.Year(), moment.Month(), moment.Day(), 0, 0, 0, 0, from.Location())
		if !at.Before(from) && at.Before(to) {
			out = append(out, obj)
		}
	}
	return out
}
