package core

import (
	"context"
	"fmt"
	"net/url"
)

// DashboardExport is a button on the filter bar: Handler builds a file from
// the current filters.
type DashboardExport struct {
	Name  string
	Label string
	// Permission defaults to "dashboard.export".
	Permission string
	Handler    func(ctx context.Context, dc DashboardContext) (*Download, error)
}

func ExportPermission(e DashboardExport) string {
	if e.Permission == "" {
		return "dashboard.export"
	}
	return e.Permission
}

// Dashboard is a separate first-class concept. The
// dashboard route is GET /admin -- independent of any single
// ModelAdmin, it's just a collection of Widgets, each deciding its own
// data and, optionally, its own extra permission.
type Dashboard struct {
	Title   string
	Widgets []Widget
	Filters []DashboardFilter
	Exports []DashboardExport
}

// VisibleWidgets returns the widgets `principal` may see: a
// widget with no permission is always shown; one that names a
// permission is simply omitted -- not shown-disabled -- if the
// authorizer denies it (or there's no authorizer to ask, in which
// case it's shown, matching the rest of the framework's
// no-authorizer-configured default of permitting everything).
func (d Dashboard) VisibleWidgets(principal *Principal, authorizer Authorizer) []Widget {
	visible := make([]Widget, 0, len(d.Widgets))
	for _, widget := range d.Widgets {
		if widget.Permission() == "" || authorizer == nil || authorizer.Can(principal, widget.Permission(), widget) {
			visible = append(visible, widget)
		}
	}
	return visible
}

func (d Dashboard) VisibleExports(principal *Principal, authorizer Authorizer) []DashboardExport {
	out := []DashboardExport{}
	for _, e := range d.Exports {
		if authorizer == nil || authorizer.Can(principal, ExportPermission(e), e) {
			out = append(out, e)
		}
	}
	return out
}

func (d Dashboard) Export(name string) (DashboardExport, bool) {
	for _, e := range d.Exports {
		if e.Name == name {
			return e, true
		}
	}
	return DashboardExport{}, false
}

func (d Dashboard) FilterNames() []string {
	names := make([]string, len(d.Filters))
	for i, f := range d.Filters {
		names[i] = f.Name()
	}
	return names
}

func requireUnique(values []string, what string) error {
	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] {
			return fmt.Errorf("dashboard has more than one %s %q", what, v)
		}
		seen[v] = true
	}
	return nil
}

func (d Dashboard) Validate() error {
	names := d.FilterNames()
	keys := make([]string, len(d.Widgets))
	for i, w := range d.Widgets {
		keys[i] = WidgetKey(w)
	}
	exports := make([]string, len(d.Exports))
	for i, e := range d.Exports {
		exports[i] = e.Name
	}
	for _, check := range []struct {
		values []string
		what   string
	}{{names, "filter name"}, {keys, "widget key"}, {exports, "export name"}} {
		if err := requireUnique(check.values, check.what); err != nil {
			return err
		}
	}
	known := map[string]bool{}
	for _, n := range names {
		known[n] = true
	}
	for _, w := range d.Widgets {
		if dw, ok := w.(DashboardWidget); ok {
			deps, _ := dw.DependsOn()
			for _, n := range deps {
				if !known[n] {
					return fmt.Errorf("dashboard widget %q depends on unknown filter %q", WidgetKey(w), n)
				}
			}
		}
		if v, ok := w.(interface{ validate() error }); ok {
			if err := v.validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d Dashboard) ParseFilters(get func(string) string) map[string]any {
	out := make(map[string]any, len(d.Filters))
	for _, f := range d.Filters {
		out[f.Name()] = f.Parse(get)
	}
	return out
}

func (d Dashboard) QueryParams(filters map[string]any) url.Values {
	values := url.Values{}
	for _, f := range d.Filters {
		for k, v := range f.QueryParams(filters[f.Name()]) {
			values.Set(k, v)
		}
	}
	return values
}

func (d Dashboard) Context(get func(string) string, principal *Principal) DashboardContext {
	return DashboardContext{Filters: d.ParseFilters(get), Principal: principal}
}

func (d Dashboard) WidgetContext(w Widget, get func(string) string, principal *Principal) WidgetContext {
	wc := WidgetContext{DashboardContext: d.Context(get, principal), Search: get("search"), Offset: parseOffset(get("offset"))}
	if p, ok := w.(Pager); ok {
		wc.Limit = p.PageSize()
	}
	return wc
}
