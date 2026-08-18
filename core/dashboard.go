package core

// Dashboard is a separate first-class concept. The
// dashboard route is GET /admin -- independent of any single
// ModelAdmin, it's just a collection of Widgets, each deciding its own
// data and, optionally, its own extra permission.
type Dashboard struct {
	Title   string
	Widgets []Widget
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
