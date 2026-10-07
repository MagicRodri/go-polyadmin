package fiber

import (
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func mountPageApp(t *testing.T, admin *core.Admin, templateDir string) *fiber.App {
	t.Helper()
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin", WithTemplateDirs(templateDir)); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app
}

func TestPageGetRendersTemplateInsideSharedLayout(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/broadcast.html", "BROADCAST FORM")

	admin := core.New()
	admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error {
		return pc.Render("pages/broadcast.html", nil)
	}), core.WithPageLabel("Broadcast Message"))
	app := mountPageApp(t, admin, dir)

	resp := doGet(t, app, "/admin/tools/broadcast", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	text := body(t, resp)
	if !strings.Contains(text, "BROADCAST FORM") {
		t.Fatalf("expected page content, got %s", text)
	}
	if !strings.Contains(text, "Dashboard") {
		t.Fatalf("expected shared layout chrome (sidebar), got %s", text)
	}
}

func TestPagePostRedirectsWithFlash(t *testing.T) {
	dir := t.TempDir()
	admin := core.New()
	admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error {
		if pc.C.Method() == fiber.MethodPost {
			return pc.RedirectWithFlash(pc.BasePath+"/tools/broadcast", "success", "Broadcast sent.")
		}
		return pc.C.SendString("form")
	}))
	app := mountPageApp(t, admin, dir)

	resp := doPostForm(t, app, "/admin/tools/broadcast", url.Values{"message": {"hi"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin/tools/broadcast" {
		t.Fatalf("got location %q", loc)
	}

	htmxResp := doPostForm(t, app, "/admin/tools/broadcast", url.Values{"message": {"hi"}}, map[string]string{"HX-Request": "true"})
	if htmxResp.StatusCode != 200 {
		t.Fatalf("got %d", htmxResp.StatusCode)
	}
	if got := htmxResp.Header.Get("HX-Redirect"); got != "/admin/tools/broadcast" {
		t.Fatalf("got HX-Redirect %q", got)
	}
}

func TestMountReturnsErrorOnWrongPageHandlerType(t *testing.T) {
	admin := core.New()
	admin.Route("/tools/broadcast", func() {}) // not a fiberadapter.PageHandler
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin"); err == nil {
		t.Fatalf("expected Mount to return an error, got nil")
	}
}

type denyBroadcastAuthorizer struct{}

func (denyBroadcastAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "page.tools.broadcast"
}

func TestPageRequestDeniedByAuthorizerReturns403(t *testing.T) {
	dir := t.TempDir()
	admin := core.New(core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)), core.WithAuthorizer(denyBroadcastAuthorizer{}))
	admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error {
		return pc.C.SendString("should not reach here")
	}))
	app := mountPageApp(t, admin, dir)

	resp := doGet(t, app, "/admin/tools/broadcast", nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestPageWithCategoryGroupsWithModelAdminInNav(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/broadcast.html", "BROADCAST FORM")

	userAdmin := newTestUserAdmin()
	userAdmin.NavCategory = "Tools"
	admin := core.New(core.WithModelAdmins(userAdmin))
	admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error {
		return pc.Render("pages/broadcast.html", nil)
	}), core.WithPageLabel("Broadcast Message"), core.WithPageCategory("Tools"))
	app := mountPageApp(t, admin, dir)

	resp := doGet(t, app, "/admin/tools/broadcast", nil)
	text := body(t, resp)
	if !strings.Contains(text, "Tools") {
		t.Fatalf("expected the shared 'Tools' category accordion label, got %s", text)
	}
	if !strings.Contains(text, "Broadcast Message") || !strings.Contains(text, "User") {
		t.Fatalf("expected both the page and the ModelAdmin nav links, got %s", text)
	}
}
