package core

import (
	"fmt"
	"io/fs"
)

// Admin is the root admin application: it owns the ModelAdmin registry.
// Route registration, template/static configuration, and Mount() are
// implemented by the framework adapters (e.g. go/polyadmin/fiber), not here.
type Admin struct {
	Dashboard     *Dashboard
	Authenticator Authenticator
	Authorizer    Authorizer
	SiteTitle     string
	SiteLogoURL   string

	// DisableCSRF turns off CSRF verification. The zero value is
	// therefore "enabled", matching the Disable* convention on
	// BaseModelAdmin -- a security control has to be opt-out, never
	// opt-in. The token cookie is still minted when this is set, so
	// templates and custom pages behave identically either way.
	DisableCSRF bool
	// AuditLogger, when set, receives an entry for every create,
	// update, delete and action. Nil means nothing is recorded -- the
	// framework does not store a log itself.
	AuditLogger AuditLogger
	// LoginBackend, when set, mounts the built-in login page and makes
	// an unauthenticated request redirect there instead of returning
	// 401. Nil leaves both behaviours off -- see login.go.
	LoginBackend LoginBackend

	// Internationalisation -- see core/i18n.go and docs/i18n.md. All
	// optional: the zero values serve English plus every framework
	// catalog, resolved per request, with the language switcher shown.
	DefaultLocale         string
	Locales               []string
	LocaleResolver        LocaleResolver
	Catalogs              []fs.FS
	Translator            Translator
	LocaleNames           map[string]string
	DisableLocaleSwitcher bool
	PseudoLocale          bool

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

// WithCSRFDisabled turns off CSRF verification, for deployments that
// already front the admin with their own protection. The clickjacking
// headers are not affected: framing is a different attack from forgery.
func WithCSRFDisabled() Option {
	return func(a *Admin) { a.DisableCSRF = true }
}

// WithAuditLogger records every change through the given logger. Where
// the entries go is the host application's decision -- see AuditLogger.
func WithAuditLogger(logger AuditLogger) Option {
	return func(a *Admin) { a.AuditLogger = logger }
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

// WithLoginBackend turns on the admin's built-in login page, backed by
// the application's own credential check and session handling. Pair it
// with the Authenticator that reads back whatever BeginSession wrote --
// the two are two halves of one arrangement. See LoginBackend.
func WithLoginBackend(backend LoginBackend) Option {
	return func(a *Admin) { a.LoginBackend = backend }
}

// WithDefaultLocale sets the locale used when nothing else decides.
func WithDefaultLocale(locale string) Option {
	return func(a *Admin) { a.DefaultLocale = locale }
}

// WithLocales restricts the supported locales, in switcher order.
func WithLocales(locales ...string) Option {
	return func(a *Admin) { a.Locales = append(a.Locales, locales...) }
}

// WithLocaleResolver lets the host choose a request's locale, e.g. from a
// preference stored on the principal. The switcher cookie still wins.
func WithLocaleResolver(resolver LocaleResolver) Option {
	return func(a *Admin) { a.LocaleResolver = resolver }
}

// WithCatalogs layers host catalogs (one <locale>.json per file, at the
// filesystem root) over the framework's.
func WithCatalogs(catalogs ...fs.FS) Option {
	return func(a *Admin) { a.Catalogs = append(a.Catalogs, catalogs...) }
}

// WithTranslator replaces the catalog-backed Translator entirely. Pair it
// with WithLocales: the admin cannot list a foreign translator's locales.
func WithTranslator(translator Translator) Option {
	return func(a *Admin) { a.Translator = translator }
}

// WithLocaleNames names host-added locales in the switcher.
func WithLocaleNames(names map[string]string) Option {
	return func(a *Admin) { a.LocaleNames = names }
}

// WithoutLocaleSwitcher hides the language switcher and unmounts its
// route -- for hosts that force a locale through a LocaleResolver. With
// it, a leftover admin_locale cookie no longer affects the locale.
func WithoutLocaleSwitcher() Option {
	return func(a *Admin) { a.DisableLocaleSwitcher = true }
}

// WithPseudoLocale enables en-XA, which renders every translated string
// accented and bracketed. For tests and development only.
func WithPseudoLocale() Option {
	return func(a *Admin) { a.PseudoLocale = true }
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
