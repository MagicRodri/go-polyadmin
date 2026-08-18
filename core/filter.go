package core

import "fmt"

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
	return [][2]string{{"", "All"}, {"true", "Yes"}, {"false", "No"}}
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
	pairs = append(pairs, [2]string{"", "All"})
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
