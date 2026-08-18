package core

import "strings"

// AdminPage is an application-registered admin route with its own
// template, rendered inside the shared admin layout (sidebar,
// breadcrumbs, flash toasts) -- for functionality that isn't resource
// CRUD (reports, wizards, internal tools). See docs/routing.md.
//
// Named AdminPage, not Page, to avoid colliding with core.Page (a
// paginated slice of list results, see pagination.go).
//
// Handler is `any`, not a concrete function type -- core must not
// import the fiber adapter, mirroring the existing
// Authenticator.Authenticate(request any) convention. Applications
// register with admin.Route(path, fiberadapter.PageHandler(fn), ...),
// an explicit wrap like http.HandlerFunc; the adapter's Mount type-asserts
// it back.
type AdminPage struct {
	Path     string
	Handler  any
	Label    string
	Category string
	// Icon names the sidebar-nav icon (see fiber/icons.go's iconPaths)
	// shown next to this page's own link. Defaults to defaultIcon.
	Icon       string
	Permission string
	// HTTPMethods defaults to {"GET", "POST"} via NewAdminPage --
	// enough for a page to render a form (GET) and repost to itself
	// (POST), the shape a multi-step wizard needs.
	HTTPMethods []string
	// HideFromNav opts a page out of the sidebar; false (the zero
	// value) means shown, matching this package's Disable* convention.
	HideFromNav bool
}

// PageOption configures an AdminPage built with NewAdminPage.
type PageOption func(*AdminPage)

func WithPageLabel(label string) PageOption {
	return func(p *AdminPage) { p.Label = label }
}

func WithPageCategory(category string) PageOption {
	return func(p *AdminPage) { p.Category = category }
}

func WithPageIcon(icon string) PageOption {
	return func(p *AdminPage) { p.Icon = icon }
}

func WithPagePermission(permission string) PageOption {
	return func(p *AdminPage) { p.Permission = permission }
}

func WithPageMethods(methods ...string) PageOption {
	return func(p *AdminPage) { p.HTTPMethods = methods }
}

func WithPageHiddenFromNav() PageOption {
	return func(p *AdminPage) { p.HideFromNav = true }
}

// NewAdminPage builds an AdminPage, deriving Label/Permission from
// path when not set explicitly via options.
func NewAdminPage(path string, handler any, opts ...PageOption) AdminPage {
	p := AdminPage{Path: path, Handler: handler, HTTPMethods: []string{"GET", "POST"}}
	for _, opt := range opts {
		opt(&p)
	}
	if p.Label == "" {
		p.Label = defaultPageLabel(path)
	}
	if p.Permission == "" {
		p.Permission = defaultPagePermission(path)
	}
	if p.Icon == "" {
		p.Icon = defaultIcon
	}
	return p
}

func defaultPageLabel(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return "Page"
	}
	parts := strings.Split(trimmed, "/")
	last := strings.ReplaceAll(strings.ReplaceAll(parts[len(parts)-1], "-", " "), "_", " ")
	return titleCase(last)
}

// defaultPagePermission mirrors ResourcePermission's "{slug}.{action}"
// shape: "/reports/contracts" -> "page.reports.contracts".
func defaultPagePermission(path string) string {
	return "page." + strings.ReplaceAll(strings.Trim(path, "/"), "/", ".")
}

// titleCase upper-cases the first letter of each whitespace-separated
// word -- strings.Title is deprecated (it mishandles Unicode word
// boundaries), but page labels are plain ASCII path segments, so this
// simple version is sufficient and dependency-free.
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
