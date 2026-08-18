package core

import (
	"sort"
	"strings"
)

// ListRequest is the query pipeline's input: search -> filters
// -> ordering. ExecuteListQuery is deliberately independent of
// pagination so the same filtered/ordered result set can back the list
// view, an export, or a custom action.
type ListRequest struct {
	Search   string
	Filters  map[string]string
	Ordering string
	Page     int
	PageSize int
}

func ApplySearch(modelAdmin ModelAdmin, objects []any, search string) []any {
	if search == "" {
		return objects
	}
	term := strings.ToLower(search)
	fields := make([]Field, 0, len(modelAdmin.SearchFields()))
	for _, name := range modelAdmin.SearchFields() {
		if field, ok := modelAdmin.Field(name); ok {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return objects
	}
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		for _, field := range fields {
			value := field.GetValue(obj)
			if value == nil {
				continue
			}
			if strings.Contains(strings.ToLower(stringify(value)), term) {
				out = append(out, obj)
				break
			}
		}
	}
	return out
}

func ApplyFilters(modelAdmin ModelAdmin, objects []any, raw map[string]string) []any {
	for _, filter := range modelAdmin.Filters() {
		if value, ok := raw[filter.Name()]; ok {
			objects = filter.Apply(objects, value, modelAdmin)
		}
	}
	return objects
}

func ApplyOrdering(modelAdmin ModelAdmin, objects []any, ordering string) []any {
	if ordering == "" {
		return objects
	}
	reverse := strings.HasPrefix(ordering, "-")
	name := ordering
	if reverse {
		name = ordering[1:]
	}
	field, ok := modelAdmin.Field(name)
	if !ok {
		return objects
	}

	sorted := append([]any(nil), objects...)
	sort.SliceStable(sorted, func(i, j int) bool {
		vi, vj := field.GetValue(sorted[i]), field.GetValue(sorted[j])
		if vi == nil {
			return false
		}
		if vj == nil {
			return true
		}
		less := lessAny(vi, vj)
		if reverse {
			return !less && stringify(vi) != stringify(vj)
		}
		return less
	})
	return sorted
}

// lessAny compares two field values across the handful of concrete
// types Fields actually hold; anything else falls back to string
// comparison so ordering never panics on an unexpected type.
func lessAny(a, b any) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av < bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av < bv
		}
	case float64:
		if bv, ok := b.(float64); ok {
			return av < bv
		}
	case bool:
		if bv, ok := b.(bool); ok {
			return !av && bv
		}
	}
	return stringify(a) < stringify(b)
}

func ExecuteListQuery(modelAdmin ModelAdmin, objects []any, req ListRequest) []any {
	objects = ApplySearch(modelAdmin, objects, req.Search)
	objects = ApplyFilters(modelAdmin, objects, req.Filters)
	objects = ApplyOrdering(modelAdmin, objects, req.Ordering)
	return objects
}
