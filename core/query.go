package core

import (
	"context"
	"net/url"
	"sort"
	"strings"
)

// ListRequest is the query pipeline's input: search -> filters ->
// ordering, independent of pagination so one result set can back the list
// view, an export, or a custom action.
type ListRequest struct {
	Search   string
	Filters  map[string]string
	Ordering string
	Page     int
	PageSize int
	// Unlimited asks for every matching row, which is what an export wants.
	// It overrides Page/PageSize rather than being PageSize 0, so "unset" and
	// "all" stay distinguishable.
	Unlimited bool
}

// DefaultPageSize is the size assumed when a request names none. It
// matches the handlers' own query-string default, so a ListRequest
// built by hand and one parsed from a URL page the same way.
const DefaultPageSize = 25

// DefaultEmptyValue stands in for a nil or blank value on the list and
// detail views -- an em dash, which reads as "nothing here" rather than
// as an empty cell that might be a rendering bug.
const DefaultEmptyValue = "\u2014"

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

// ListQuerier is an optional ModelAdmin capability: implement it to
// resolve search, filters, ordering and the page window in the data source
// itself, rather than in memory over everything GetQueryset returns.
//
// All-or-nothing by design. The framework applies nothing further, because
// it cannot tell what the implementation already did and re-applying would
// double-filter. The returned total counts rows matching search+filters
// before the window. Not implementing it keeps the in-memory path.
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

// IsSortable reports whether a list column offers a sort. Unset
// SortableFields leaves every column sortable; an empty one leaves none.
func IsSortable(modelAdmin ModelAdmin, name string) bool {
	sortable := modelAdmin.SortableFields()
	if sortable == nil {
		return true
	}
	for _, allowed := range sortable {
		if allowed == name {
			return true
		}
	}
	return false
}

// LinksToRecord reports whether a list cell links to the record. Unset
// LinkFields links the first column, as Django does; an empty one links
// nothing and leaves the row menu as the way in.
func LinksToRecord(modelAdmin ModelAdmin, name string) bool {
	linked := modelAdmin.LinkFields()
	if linked == nil {
		display := modelAdmin.ListDisplay()
		return len(display) > 0 && display[0] == name
	}
	for _, allowed := range linked {
		if allowed == name {
			return true
		}
	}
	return false
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

// ApplyDefaults fills in the ModelAdmin's ordering and page size where the
// request named neither. Resolved before the query runs so a ListQuerier
// is told about them too: they are part of the question, not the answer.
// Idempotent, so a caller needing the resolved values can apply it once
// and pass the same request on.
func ApplyDefaults(modelAdmin ModelAdmin, req ListRequest) ListRequest {
	// A ?sort= naming a column the admin does not offer is dropped, so
	// the restriction holds for a hand-typed URL too. DefaultOrdering
	// below is exempt: it is the admin's own choice, not user input.
	if req.Ordering != "" && !IsSortable(modelAdmin, strings.TrimPrefix(req.Ordering, "-")) {
		req.Ordering = ""
	}
	if req.Ordering == "" {
		req.Ordering = modelAdmin.DefaultOrdering()
	}
	// Not for an unlimited request: that deliberately has no page.
	if !req.Unlimited && req.PageSize < 1 {
		req.PageSize = modelAdmin.PageSize()
	}
	return req
}

// ListObjects resolves a list query and is the only place that decides
// how: a core.ListQuerier answers it itself, everything else loads the
// queryset and filters in memory.
//
// Every consumer goes through here -- list view, both exports, the
// autocomplete lookup, relation option lists -- so the paths cannot drift;
// the request's Window is what separates "one page" from "capped at 20"
// from "every matching row". Returns the window's objects and the total
// before it.
func ListObjects(ctx context.Context, modelAdmin ModelAdmin, req ListRequest) ([]any, int, error) {
	req = ApplyDefaults(modelAdmin, req)
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

// ListTokenField is the reserved query parameter and form field carrying
// the list a page was reached from -- its search, filters, sort and page
// (docs/lists.md). It is what preserve_filters preserves: the pages
// reached from a list hand it back, so the trail out of a filtered list
// leads into it rather than into the bare one.
const ListTokenField = "_list"

// SafeListToken validates a token the way SafeRedirectPath validates a
// Referer: same host, under the admin's base path, no control
// characters. An invalid one yields "", which every caller reads as "no
// list to go back to".
func SafeListToken(token, host, basePath string) string {
	if token == "" {
		return ""
	}
	if safe := SafeRedirectPath(token, host, basePath, ""); safe != "" {
		return safe
	}
	return ""
}

// WithListToken appends a validated token to a URL as the _list
// parameter, and returns the URL unchanged when there is none.
func WithListToken(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + ListTokenField + "=" + url.QueryEscape(token)
}
