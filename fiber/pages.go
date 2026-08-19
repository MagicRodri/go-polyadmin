package fiber

import (
	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// PageContext is what an AdminPage's handler receives -- the raw
// *fiber.Ctx (for form/query parsing, exactly like any other Fiber
// handler) plus render/redirect helpers reusing the framework's own
// layout, flash cookie, and HTMX-aware redirect.
type PageContext struct {
	C         *fiber.Ctx
	Admin     *core.Admin
	Page      core.AdminPage
	Principal *core.Principal
	BasePath  string

	renderer *Renderer
}

func (pc *PageContext) IsHTMX() bool { return isHTMXRequest(pc.C) }

// Render renders templateName (an application-supplied template
// defining a "content" block, resolved via Renderer.PageTemplate)
// inside the shared admin layout, popping and clearing any pending
// flash message the same way the framework's own views do.
func (pc *PageContext) Render(templateName string, data any) error {
	html, err := pc.renderer.RenderPage(pc.Principal, pc.Page, templateName, data, popFlash(pc.C))
	if err != nil {
		return err
	}
	clearFlash(pc.C)
	pc.C.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return pc.C.SendString(html)
}

// Redirect is the framework's HTMX-aware redirect (see redirectTo) --
// use this for a plain redirect and RedirectWithFlash when the target
// page should show a toast.
func (pc *PageContext) Redirect(url string) error { return redirectTo(pc.C, url) }

func (pc *PageContext) RedirectWithFlash(url, level, text string) error {
	setFlash(pc.C, level, text)
	return redirectTo(pc.C, url)
}

// PageHandler is the signature applications implement for a custom
// admin page -- registered via
// admin.Route(path, fiberadapter.PageHandler(yourFunc), ...), the
// same explicit-wrap idiom as http.HandlerFunc.
type PageHandler func(pc *PageContext) error

// buildPageHandler wires one AdminPage into a fiber.Handler: authorize
// against the page's declared permission, then hand off to the
// application's PageHandler. page is passed by value (not closed over
// a loop variable) to sidestep the classic Go range-closure footgun,
// matching handleList et al.'s pattern of taking modelAdmin as a
// parameter.
func buildPageHandler(admin *core.Admin, page core.AdminPage, renderer *Renderer, basePath string, handler PageHandler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, page.Permission, page)
		if result != authOK {
			return writeAuthError(c, result)
		}
		pc := &PageContext{
			C: c, Admin: admin, Page: page, Principal: principal, BasePath: basePath,
			renderer: renderer,
		}
		return handler(pc)
	}
}
