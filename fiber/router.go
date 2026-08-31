package fiber

import (
	"fmt"

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

	// Before every route, including the static handler and any custom
	// AdminPage: a mutating custom page has to be covered too.
	router.Use(csrfMiddleware(admin, basePath))

	if cfg.staticDir != "" {
		router.Static("/static", cfg.staticDir)
	}

	renderer, err := NewRenderer(admin, basePath, cfg.templateDirs...)
	if err != nil {
		return err
	}

	router.Get("/", func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.DashboardView, nil)
		if result != authOK {
			return writeAuthError(c, result)
		}
		if admin.Dashboard != nil {
			widgets := admin.Dashboard.VisibleWidgets(principal, admin.Authorizer)
			html, err := renderer.RenderDashboard(principal, *admin.Dashboard, widgets)
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
		return c.SendString("<p>No resources registered.</p>")
	})

	for _, modelAdmin := range admin.ModelAdmins() {
		prefix := "/" + modelAdmin.Slug()

		if modelAdmin.CanView() {
			router.Get(prefix, handleList(admin, modelAdmin, renderer, basePath))
		}
		if modelAdmin.CanCreate() {
			router.Get(prefix+"/create", handleCreateGet(admin, modelAdmin, renderer, basePath))
			router.Post(prefix+"/create", handleCreatePost(admin, modelAdmin, renderer, basePath))
		}
		if modelAdmin.CanExport() {
			router.Get(prefix+"/export/csv", handleExportCSV(admin, modelAdmin, basePath))
			router.Get(prefix+"/export/xlsx", handleExportXLSX(admin, modelAdmin, basePath))
		}
		if modelAdmin.CanView() {
			router.Get(prefix+"/lookup", handleLookup(admin, modelAdmin, renderer))
			if len(modelAdmin.Actions()) > 0 {
				router.Post(prefix+"/actions/:name", handleAction(admin, modelAdmin, basePath))
			}
			router.Get(prefix+"/:pk", handleDetail(admin, modelAdmin, renderer, basePath))
		}
		if modelAdmin.CanUpdate() {
			router.Get(prefix+"/:pk/edit", handleEditGet(admin, modelAdmin, renderer, basePath))
			router.Post(prefix+"/:pk/edit", handleEditPost(admin, modelAdmin, renderer, basePath))
		}
		if modelAdmin.CanDelete() {
			router.Get(prefix+"/:pk/delete", handleDeleteGet(admin, modelAdmin, renderer, basePath))
			router.Post(prefix+"/:pk/delete", handleDeletePost(admin, modelAdmin, basePath))
			router.Delete(prefix+"/:pk/delete", handleDeleteHTMX(admin, modelAdmin))
		}

		if len(modelAdmin.Inlines()) > 0 {
			if err := validateInlines(admin, modelAdmin); err != nil {
				return err
			}
			router.Post(prefix+"/:pk/inlines/:child", handleInlineCreate(admin, modelAdmin, renderer, basePath))
			router.Post(prefix+"/:pk/inlines/:child/:childPK", handleInlineUpdate(admin, modelAdmin, renderer, basePath))
			router.Delete(prefix+"/:pk/inlines/:child/:childPK", handleInlineDelete(admin, modelAdmin, renderer, basePath))
		}
	}

	for _, page := range admin.Pages() {
		handler, ok := page.Handler.(PageHandler)
		if !ok {
			return fmt.Errorf("polyadmin: page %q's handler must be a fiberadapter.PageHandler (wrap it: fiberadapter.PageHandler(yourFunc))", page.Path)
		}
		fiberHandler := buildPageHandler(admin, page, renderer, basePath, handler)
		for _, method := range page.HTTPMethods {
			router.Add(method, page.Path, fiberHandler)
		}
	}

	return nil
}
