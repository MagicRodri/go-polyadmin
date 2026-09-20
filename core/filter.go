package core

import (
	"fmt"
	"reflect"
	"strings"
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

// DateFilterRangeLayout is the date layout a custom range uses, in the
// URL and in the panel's two inputs. It is the same one <input
// type="date"> posts, so the form needs no reformatting.
const DateFilterRangeLayout = "2006-01-02"

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

	// Not a preset, so try a custom range: "<from>:<to>", each
	// YYYY-MM-DD. It is inclusive of both endpoints, because that is what
	// "from X to Y" means to a reader, and is converted here to the same
	// half-open window the presets produce.
	start, end, isRange := strings.Cut(raw, ":")
	if !isRange || start == "" {
		// An empty start is rejected: "through today" has a natural
		// resolution, "since the beginning of time" does not.
		return time.Time{}, time.Time{}, false
	}
	fromDay, err := time.ParseInLocation(DateFilterRangeLayout, start, now.Location())
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	// An empty end means "through today", resolved here rather than in
	// the panel: the form posts the blank when scripting is off, and a
	// value the parser rejected would break the one path the panel's
	// links exist to protect.
	toDay := day
	if end != "" {
		toDay, err = time.ParseInLocation(DateFilterRangeLayout, end, now.Location())
		if err != nil {
			return time.Time{}, time.Time{}, false
		}
	}
	if toDay.Before(fromDay) {
		return time.Time{}, time.Time{}, false
	}
	return fromDay, toDay.AddDate(0, 0, 1), true
}

// DateFilterRangeValues splits a custom range back into the two strings
// it carries, so the panel can prefill its inputs. It parses nothing: ok
// is false for a preset and for anything without a separator.
func DateFilterRangeValues(raw string) (from, to string, ok bool) {
	start, end, isRange := strings.Cut(raw, ":")
	if !isRange || start == "" {
		return "", "", false
	}
	return start, end, true
}

// ControlKind makes DateFilter a FilterControl: its own choices are the
// presets, and the panel draws two date inputs below them.
func (f DateFilter) ControlKind() FilterKind { return FilterKindDateRange }

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

// FilterKind is how a filter's control is sourced and drawn. It answers
// two questions at once -- who supplies the choices, and what the panel
// renders -- because they are the same decision: a filter that cannot
// enumerate its own values is exactly the filter that needs a control
// other than a list of links.
type FilterKind string

const (
	// FilterKindChoices is the default and is never declared: the filter
	// supplies its own choices and the panel draws links.
	FilterKindChoices FilterKind = ""
	// FilterKindRelation means the adapter supplies the choices, from the
	// target ModelAdmin -- ChoicesWithLabels cannot, because it reaches
	// neither the registry nor the principal.
	FilterKindRelation FilterKind = "relation"
	// FilterKindDateRange means the filter's own choices are presets, and
	// the panel adds two date inputs below them.
	FilterKindDateRange FilterKind = "daterange"
)

// FilterControl is an optional capability, like DeletePreviewer and
// ListQuerier. A filter that does not implement it is a plain choice
// list -- which is every filter written before this existed.
type FilterControl interface {
	ControlKind() FilterKind
}

// The empty filter's values. Stable strings, because they end up in URLs.
const (
	EmptyFilterEmpty    = "empty"
	EmptyFilterNotEmpty = "notempty"
)

// IsEmptyValue reports whether a field's value counts as empty: unset or
// blank, never merely zero. nil, "", an empty collection, a nil pointer
// and a zero time are empty; 0, false and "0" are values somebody chose,
// and are not.
//
// A plain int or bool field can therefore never be empty, which is
// correct -- those types cannot represent "unset". A host that needs a
// nullable number uses *int, as it already must for anything optional.
func IsEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	if moment, isTime := asTime(value); isTime {
		return moment.IsZero()
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		// A nil pointer is an absence; a pointer to something is not,
		// whatever it points at -- *int(0) is a chosen zero.
		return v.IsNil()
	default:
		return false
	}
}

// EmptyFilter splits a list on whether a field has a value at all. It
// applies to relation fields too -- "Organization is empty" is the case
// that earns it -- where a `many` relation is empty when it has no
// members.
type EmptyFilter struct {
	baseFilter
}

func NewEmptyFilter(name string, opts ...func(*string)) EmptyFilter {
	var label string
	for _, opt := range opts {
		opt(&label)
	}
	return EmptyFilter{newBaseFilter(name, label)}
}

func (f EmptyFilter) ChoicesWithLabels() [][2]string {
	return [][2]string{
		{"", N_("All")},
		{EmptyFilterEmpty, N_("Empty")},
		{EmptyFilterNotEmpty, N_("Not empty")},
	}
}

func (f EmptyFilter) Apply(objects []any, raw string, modelAdmin ModelAdmin) []any {
	var wantEmpty bool
	switch raw {
	case EmptyFilterEmpty:
		wantEmpty = true
	case EmptyFilterNotEmpty:
		wantEmpty = false
	default:
		// "" and anything crafted narrow nothing.
		return objects
	}
	field, ok := modelAdmin.Field(f.name)
	if !ok {
		return objects
	}
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		if IsEmptyValue(field.GetValue(obj)) == wantEmpty {
			out = append(out, obj)
		}
	}
	return out
}

// RelationFilter narrows a list to rows pointing at one related record.
// The URL carries the target's primary key, so a filtered list is a link
// like any other.
//
// Its choices come from the adapter, not from here: ChoicesWithLabels
// reaches neither the registry nor the principal, and a filter must not
// offer records from a target the reader may not view.
type RelationFilter struct {
	baseFilter
	// RelatedPK resolves the related object's primary key. It defaults to
	// the lookup BaseModelAdmin.GetPK uses when no PK func is declared.
	// Set it when the target declares its own: Apply receives the parent
	// ModelAdmin, not the registry, and Relation.Target is a slug, so
	// core cannot call the target's GetPK for you.
	RelatedPK func(related any) any
}

func NewRelationFilter(name string, opts ...func(*string)) RelationFilter {
	var label string
	for _, opt := range opts {
		opt(&label)
	}
	return RelationFilter{baseFilter: newBaseFilter(name, label)}
}

// ControlKind makes RelationFilter a FilterControl: the adapter sources
// its choices, and renders them as links or as the lookup-backed
// combobox depending on AutocompleteFields.
func (f RelationFilter) ControlKind() FilterKind { return FilterKindRelation }

func (f RelationFilter) ChoicesWithLabels() [][2]string {
	return [][2]string{{"", N_("All")}}
}

func (f RelationFilter) relatedPK(related any) string {
	if related == nil {
		return ""
	}
	resolve := f.RelatedPK
	if resolve == nil {
		resolve = defaultPK
	}
	pk := resolve(related)
	if pk == nil {
		return ""
	}
	return stringify(pk)
}

func (f RelationFilter) Apply(objects []any, raw string, modelAdmin ModelAdmin) []any {
	if raw == "" {
		return objects
	}
	field, ok := modelAdmin.Field(f.name)
	if !ok || field.Relation == nil {
		return objects
	}
	many := field.Relation.Cardinality == CardinalityMany
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		related := field.Relation.GetValue(obj)
		if related == nil {
			continue
		}
		if !many {
			if f.relatedPK(related) == raw {
				out = append(out, obj)
			}
			continue
		}
		// A `many` relation matches when any member does. The value is
		// walked by reflection rather than asserted to []any: a host's
		// field may hold a typed slice, and an assertion would silently
		// match nothing.
		members := reflect.ValueOf(related)
		if members.Kind() != reflect.Slice && members.Kind() != reflect.Array {
			continue
		}
		for i := 0; i < members.Len(); i++ {
			if f.relatedPK(members.Index(i).Interface()) == raw {
				out = append(out, obj)
				break
			}
		}
	}
	return out
}
