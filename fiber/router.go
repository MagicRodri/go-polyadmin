package fiber

import (
	"fmt"
	"html"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// MountOption configures optional Mount behavior -- the functional-
// options equivalent of the FastAPI adapter's create_router keyword
// arguments (template_dirs, static_dir).
type MountOption func(*mountConfig)

type mountConfig struct {
	staticDir    string
	templateDirs []string
}

// WithStaticDir serves an application-supplied directory at
// <basePath>/static/* (e.g. a custom.css, a locally-hosted logo image)
// -- mirrors the FastAPI adapter's create_router(..., static_dir=...).
// Framework-owned static assets don't exist in either language today
// since Tailwind/Alpine/HTMX are all CDN-loaded; this is purely an
// extension point for an application's own files.
func WithStaticDir(dir string) MountOption {
	return func(c *mountConfig) { c.staticDir = dir }
}

// WithTemplateDirs adds application-supplied template override
// directories, searched in the order given, before the framework's
// own embedded templates -- mirrors the FastAPI adapter's
// create_router(..., template_dirs=...). See docs/templates.md.
func WithTemplateDirs(dirs ...string) MountOption {
	return func(c *mountConfig) { c.templateDirs = append(c.templateDirs, dirs...) }
}

// Mount registers an Admin's routes on a Fiber app or group:
//
//	group := app.Group("/admin")
//	fiberadapter.Mount(group, admin, "/admin")
//
// basePath must match the prefix the group was created under -- it's
// used to build the links rendered inside templates (nav, pagination,
// redirects). The two aren't derived from each other because Fiber
// doesn't expose a group's own prefix back to route handlers.
func Mount(router fiber.Router, admin *core.Admin, basePath string, opts ...MountOption) error {
	cfg := &mountConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	i18n, err := core.NewI18n(admin)
	if err != nil {
		return err
	}
	renderers, err := NewRenderers(admin, i18n, basePath, cfg.templateDirs...)
	if err != nil {
		return err
	}

	// Locale first, then CSRF: a CSRF failure page is rendered in the
	// request's language like every other page.
	staticPrefix := ""
	if cfg.staticDir != "" {
		staticPrefix = basePath + "/static/"
	}
	router.Use(localeMiddleware(admin, i18n, renderers, staticPrefix))
	// Before every route, including the static handler and any custom
	// AdminPage: a mutating custom page has to be covered too.
	router.Use(csrfMiddleware(admin, basePath))

	if cfg.staticDir != "" {
		router.Static("/static", cfg.staticDir)
	}

	// The login routes go on before anything else, and only when the
	// application supplied a backend to make them work. They are the two
	// routes that do not authenticate -- requiring a session to reach
	// the page that creates one is a loop -- so they are also the two
	// that must not be shadowed by a ModelAdmin whose slug happens to be
	// "login". Registering them first is what makes that collision
	// Fiber's problem rather than a silently unreachable login page.
	if admin.LoginBackend != nil {
		router.Get(core.LoginPath, handleLoginGet(admin, renderers, basePath))
		router.Post(core.LoginPath, handleLoginPost(admin, renderers, basePath))
		router.Post(core.LogoutPath, handleLogout(admin, basePath))
	}

	// Mounted before any ModelAdmin route, for the same reason as the
	// login routes above: a slug of "locale" must not be able to shadow
	// it. Present only when the switcher itself would render something.
	if switcherFor(admin, i18n, i18n.Default) != nil {
		router.Post(core.LocalePath, handleLocalePost(i18n, basePath))
	}

	router.Get("/", func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.DashboardView, nil)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		if admin.Dashboard != nil {
			widgets := admin.Dashboard.VisibleWidgets(principal, admin.Authorizer)
			html, err := renderers.For(c).RenderDashboard(principal, csrfToken(c), *admin.Dashboard, widgets)
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.SendString(html)
		}
		for _, modelAdmin := range admin.ModelAdmins() {
			if modelAdmin.CanView() {
				return c.Redirect(basePath+"/"+modelAdmin.Slug(), fiber.StatusTemporaryRedirect)
			}
		}
		return c.SendString("<p>" + html.EscapeString(tr(c, "No resources registered.")) + "</p>")
	})

	for _, modelAdmin := range admin.ModelAdmins() {
		prefix := "/" + modelAdmin.Slug()

		if modelAdmin.CanView() {
			router.Get(prefix, handleList(admin, modelAdmin, renderers, basePath))
		}
		if modelAdmin.CanCreate() {
			router.Get(prefix+"/create", handleCreateGet(admin, modelAdmin, renderers, basePath))
			router.Post(prefix+"/create", handleCreatePost(admin, modelAdmin, renderers, basePath))
		}
		if modelAdmin.CanExport() {
			router.Get(prefix+"/export/csv", handleExportCSV(admin, modelAdmin, basePath))
			router.Get(prefix+"/export/xlsx", handleExportXLSX(admin, modelAdmin, basePath))
		}
		if modelAdmin.CanView() {
			router.Get(prefix+"/lookup", handleLookup(admin, modelAdmin, renderers, basePath))
			if len(modelAdmin.Actions()) > 0 {
				router.Post(prefix+"/actions/:name", handleAction(admin, modelAdmin, basePath))
			}
			router.Get(prefix+"/:pk", handleDetail(admin, modelAdmin, renderers, basePath))
		}
		if modelAdmin.CanUpdate() {
			router.Get(prefix+"/:pk/edit", handleEditGet(admin, modelAdmin, renderers, basePath))
			router.Post(prefix+"/:pk/edit", handleEditPost(admin, modelAdmin, renderers, basePath))
		}
		if modelAdmin.CanDelete() {
			router.Get(prefix+"/:pk/delete", handleDeleteGet(admin, modelAdmin, renderers, basePath))
			router.Post(prefix+"/:pk/delete", handleDeletePost(admin, modelAdmin, basePath))
			router.Delete(prefix+"/:pk/delete", handleDeleteHTMX(admin, modelAdmin, basePath))
		}

		if len(modelAdmin.Inlines()) > 0 {
			if err := validateInlines(admin, modelAdmin); err != nil {
				return err
			}
			router.Post(prefix+"/:pk/inlines/:child", handleInlineCreate(admin, modelAdmin, renderers, basePath))
			router.Post(prefix+"/:pk/inlines/:child/:childPK", handleInlineUpdate(admin, modelAdmin, renderers, basePath))
			router.Delete(prefix+"/:pk/inlines/:child/:childPK", handleInlineDelete(admin, modelAdmin, renderers, basePath))
		}
	}

	for _, page := range admin.Pages() {
		handler, ok := page.Handler.(PageHandler)
		if !ok {
			return fmt.Errorf("polyadmin: page %q's handler must be a fiberadapter.PageHandler (wrap it: fiberadapter.PageHandler(yourFunc))", page.Path)
		}
		fiberHandler := buildPageHandler(admin, page, renderers, basePath, handler)
		for _, method := range page.HTTPMethods {
			router.Add(method, page.Path, fiberHandler)
		}
	}

	return nil
}
