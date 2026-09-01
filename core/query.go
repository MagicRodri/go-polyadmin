package core

import (
	"context"
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
	// Unlimited asks for every matching row rather than one page --
	// what an export wants. It overrides Page/PageSize rather than
	// being expressed as PageSize 0, so "unset" and "all" stay
	// distinguishable.
	Unlimited bool
}

// DefaultPageSize is the size assumed when a request names none. It
// matches the handlers' own query-string default, so a ListRequest
// built by hand and one parsed from a URL page the same way.
const DefaultPageSize = 25

// Window converts the request's page into the (offset, limit) pair a
// data source wants. A limit of 0 means no limit -- see Unlimited.
func (r ListRequest) Window() (offset, limit int) {
	if r.Unlimited {
		return 0, 0
	}
	size := r.PageSize
	if size < 1 {
		size = DefaultPageSize
	}
	page := r.Page
	if page < 1 {
		page = 1
	}
	return (page - 1) * size, size
}

// ListQuerier is an optional ModelAdmin capability. Implement it to
// resolve the whole list query -- search, filters, ordering and the
// page window -- in the data source itself, typically as one SQL query,
// instead of letting the framework do it in memory over everything
// GetQueryset returns.
//
// It is all-or-nothing by design: when a ModelAdmin implements this,
// the framework applies *nothing* further, because it cannot tell what
// the implementation already did and re-applying would double-filter.
// The returned total is the count of rows matching search+filters
// before the window, which is what pagination displays.
//
// A ModelAdmin that does not implement it keeps the in-memory path,
// unchanged.
type ListQuerier interface {
	ListPage(ctx context.Context, req ListRequest) (objects []any, total int, err error)
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

// ListObjects resolves a list query, and is the only place that decides
// how. A ModelAdmin implementing core.ListQuerier answers it itself --
// one query in its own data source, with nothing re-applied here,
// because we cannot tell what it already did. Everything else falls
// back to loading the queryset and filtering it in memory.
//
// Every consumer goes through here (list view, both exports, the
// autocomplete lookup, relation option lists), so the two paths cannot
// drift: the request's Window is what distinguishes "one page" from
// "capped at 20" from "every matching row".
//
// Returns the objects for the requested window and the total matching
// rows before it, which is what pagination needs.
func ListObjects(ctx context.Context, modelAdmin ModelAdmin, req ListRequest) ([]any, int, error) {
	// Resolved here rather than inside the in-memory branch so a
	// ListQuerier is told about it too -- it is part of the question,
	// not part of the answer.
	if req.Ordering == "" {
		req.Ordering = modelAdmin.DefaultOrdering()
	}
	if querier, ok := modelAdmin.(ListQuerier); ok {
		return querier.ListPage(ctx, req)
	}
	value, err := modelAdmin.GetQueryset(ctx)
	if err != nil {
		return nil, 0, err
	}
	// Collections are []any by convention -- see the ModelAdmin docs.
	objects, _ := value.([]any)
	objects = ExecuteListQuery(modelAdmin, objects, req)
	total := len(objects)
	offset, limit := req.Window()
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return objects[offset:end], total, nil
}
