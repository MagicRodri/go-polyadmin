package core

import (
	"context"
	"strconv"
)

type DashboardContext struct {
	Filters   map[string]any
	Principal *Principal
}

func (c DashboardContext) DateRange(name string) DateRange {
	r, _ := c.Filters[name].(DateRange)
	return r
}

func (c DashboardContext) String(name string) string {
	s, _ := c.Filters[name].(string)
	return s
}

type WidgetContext struct {
	DashboardContext
	Search string
	Offset int
	// Limit is the widget's page size; 0 means unpaged.
	Limit int
}

// ContextWidget is a widget whose data depends on the request: it is
// loaded from its fragment route rather than rendered with the page.
type ContextWidget interface {
	Data(ctx context.Context, wc WidgetContext) (any, error)
}

// Pager is a widget that fetches its rows a page at a time.
type Pager interface {
	PageSize() int
}

// WidgetUnavailable, returned by a widget's data function, shows its
// unavailable state with Message, e.g. when the service behind it is down.
type WidgetUnavailable struct {
	Message string
}

func (e *WidgetUnavailable) Error() string { return "polyadmin: widget unavailable: " + e.Message }

// DashboardWidget is the optional side of a widget that the dashboard
// reads; every built-in widget implements it through baseWidget.
type DashboardWidget interface {
	Key() string
	DependsOn() (names []string, set bool)
	Description(dc DashboardContext) string
	EmptyText() string
}

func WidgetKey(w Widget) string {
	if dw, ok := w.(DashboardWidget); ok && dw.Key() != "" {
		return dw.Key()
	}
	return Slugify(w.Title())
}

func WidgetReloadsOn(w Widget, filterNames []string) []string {
	dw, ok := w.(DashboardWidget)
	if !ok {
		return append([]string{}, filterNames...)
	}
	names, set := dw.DependsOn()
	if !set {
		return append([]string{}, filterNames...)
	}
	known := map[string]bool{}
	for _, n := range filterNames {
		known[n] = true
	}
	out := []string{}
	for _, n := range names {
		if known[n] {
			out = append(out, n)
		}
	}
	return out
}

func WidgetDescription(w Widget, dc DashboardContext) string {
	if dw, ok := w.(DashboardWidget); ok {
		return dw.Description(dc)
	}
	return ""
}

func WidgetEmptyText(w Widget) string {
	if dw, ok := w.(DashboardWidget); ok {
		return dw.EmptyText()
	}
	return ""
}

func IsLazyWidget(w Widget) bool {
	if l, ok := w.(interface{ IsLazy() bool }); ok {
		return l.IsLazy()
	}
	_, ok := w.(ContextWidget)
	return ok
}

func ResolveWidgetData(ctx context.Context, w Widget, wc WidgetContext) (any, error) {
	if cw, ok := w.(ContextWidget); ok {
		return cw.Data(ctx, wc)
	}
	return w.GetData(), nil
}

func parseOffset(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func WidgetPlacement(w Widget) string {
	if p, ok := w.(interface{ Placement() string }); ok {
		return p.Placement()
	}
	return "grid"
}
