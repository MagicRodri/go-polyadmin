package core

import "fmt"

// Admin is the root admin application: it owns the ModelAdmin registry.
// Route registration, template/static configuration, and Mount() are
// implemented by the framework adapters (e.g. go/polyadmin/fiber), not here.
type Admin struct {
	Dashboard     *Dashboard
	Authenticator Authenticator
	Authorizer    Authorizer
	SiteTitle     string
	SiteLogoURL   string

	registry map[string]ModelAdmin
	order    []string

	pageRegistry map[string]AdminPage
	pageOrder    []string
}

// Option configures an Admin built with New.
type Option func(*Admin)

func New(opts ...Option) *Admin {
	a := &Admin{
		registry:     make(map[string]ModelAdmin),
		pageRegistry: make(map[string]AdminPage),
		SiteTitle:    "PolyAdmin",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithSiteTitle(title string) Option {
	return func(a *Admin) { a.SiteTitle = title }
}

func WithSiteLogoURL(url string) Option {
	return func(a *Admin) { a.SiteLogoURL = url }
}

func WithModelAdmins(modelAdmins ...ModelAdmin) Option {
	return func(a *Admin) {
		for _, ma := range modelAdmins {
			a.Register(ma)
		}
	}
}

func WithDashboard(dashboard *Dashboard) Option {
	return func(a *Admin) { a.Dashboard = dashboard }
}

func WithAuthenticator(authenticator Authenticator) Option {
	return func(a *Admin) { a.Authenticator = authenticator }
}

func WithAuthorizer(authorizer Authorizer) Option {
	return func(a *Admin) { a.Authorizer = authorizer }
}

// Register adds a ModelAdmin to the registry, keyed by its Slug().
// It panics on a duplicate slug, matching the standard library
// convention for registration-time programmer errors (e.g.
// http.ServeMux.Handle).
func (a *Admin) Register(modelAdmin ModelAdmin) {
	slug := modelAdmin.Slug()
	if _, exists := a.registry[slug]; exists {
		panic(fmt.Sprintf("polyadmin: a ModelAdmin is already registered for slug %q", slug))
	}
	a.registry[slug] = modelAdmin
	a.order = append(a.order, slug)
}

// GetModelAdmin looks up a registered ModelAdmin by slug.
func (a *Admin) GetModelAdmin(slug string) (ModelAdmin, bool) {
	ma, ok := a.registry[slug]
	return ma, ok
}

// ModelAdmins returns all registered ModelAdmins in registration order.
func (a *Admin) ModelAdmins() []ModelAdmin {
	out := make([]ModelAdmin, 0, len(a.order))
	for _, slug := range a.order {
		out = append(out, a.registry[slug])
	}
	return out
}

// Route registers a custom admin page -- for functionality that isn't
// resource CRUD (reports, wizards, internal tools). It panics on a
// duplicate path, matching Register's duplicate-slug behavior. See
// docs/routing.md.
func (a *Admin) Route(path string, handler any, opts ...PageOption) AdminPage {
	page := NewAdminPage(path, handler, opts...)
	if _, exists := a.pageRegistry[page.Path]; exists {
		panic(fmt.Sprintf("polyadmin: a page is already registered for path %q", page.Path))
	}
	a.pageRegistry[page.Path] = page
	a.pageOrder = append(a.pageOrder, page.Path)
	return page
}

// Pages returns all registered AdminPages in registration order.
func (a *Admin) Pages() []AdminPage {
	out := make([]AdminPage, 0, len(a.pageOrder))
	for _, path := range a.pageOrder {
		out = append(out, a.pageRegistry[path])
	}
	return out
}
