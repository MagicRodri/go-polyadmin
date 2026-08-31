package fiber

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// writeOverrideTemplate creates dir/relPath with a minimal Go template
// defining "content" -- override files only need to supply that block;
// "base" always comes from the framework's own base.html.
func writeOverrideTemplate(t *testing.T, dir, relPath, marker string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{{define "content"}}` + marker + `{{end}}`
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func newAppWithTemplateDirs(t *testing.T, userAdmin *testUserAdmin, dirs ...string) *fiber.App {
	t.Helper()
	admin := core.New(core.WithModelAdmins(userAdmin))
	app := fiber.New()
	group := app.Group("/admin")
	opts := make([]MountOption, len(dirs))
	for i, d := range dirs {
		opts[i] = WithTemplateDirs(d)
	}
	if err := Mount(group, admin, "/admin", opts...); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app
}

func TestApplicationTemplateDirOverridesFrameworkDefault(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "admin/list.html", "CUSTOM LIST TEMPLATE")

	app := newAppWithTemplateDirs(t, newTestUserAdmin(), dir)
	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, "CUSTOM LIST TEMPLATE") {
		t.Fatalf("expected override content, got %s", text)
	}
}

func TestResourceSpecificTemplateBeatsGenericDefault(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "admin/resource/users/list.html", "USERS-ONLY LIST TEMPLATE")

	app := newAppWithTemplateDirs(t, newTestUserAdmin(), dir)
	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, "USERS-ONLY LIST TEMPLATE") {
		t.Fatalf("expected resource-specific override, got %s", text)
	}
}

func TestExplicitOverrideBeatsResourceSpecific(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "admin/resource/users/list.html", "RESOURCE SPECIFIC")
	writeOverrideTemplate(t, dir, "custom/my-list.html", "EXPLICIT OVERRIDE WINS")

	userAdmin := newTestUserAdmin()
	userAdmin.ListTemplate = "custom/my-list.html"

	app := newAppWithTemplateDirs(t, userAdmin, dir)
	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, "EXPLICIT OVERRIDE WINS") {
		t.Fatalf("expected explicit override to win, got %s", text)
	}
	if strings.Contains(text, "RESOURCE SPECIFIC") {
		t.Fatalf("resource-specific override should have been skipped, got %s", text)
	}
}

func TestNoOverrideConfiguredUsesFrameworkDefault(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)
	resp := doGet(t, app, "/admin/users", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "john@example.com") {
		t.Fatalf("expected the normal list view, got %s", text)
	}
}

// customWidget is a minimal core.Widget implementation for exercising
// widgetTemplate's application-supplied-template path -- core.Widget's
// built-in constructors (NewMetric, etc.) all point at framework
// templates, so a custom one has to implement the interface directly.
type customWidget struct{ template string }

func (w customWidget) Title() string      { return "Custom" }
func (w customWidget) Size() string       { return "md" }
func (w customWidget) Permission() string { return "" }
func (w customWidget) Template() string   { return w.template }
func (w customWidget) GetData() any       { return nil }

func TestCustomWidgetTemplateResolvedFromTemplateDir(t *testing.T) {
	dir := t.TempDir()
	full := filepath.Join(dir, "custom", "hello.html")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := `{{define "custom/hello.html"}}HELLO FROM CUSTOM WIDGET{{end}}`
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	dashboard := &core.Dashboard{Widgets: []core.Widget{customWidget{template: "custom/hello.html"}}}
	admin := core.New(core.WithDashboard(dashboard))
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin", WithTemplateDirs(dir)); err != nil {
		t.Fatalf("mount: %v", err)
	}

	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "HELLO FROM CUSTOM WIDGET") {
		t.Fatalf("got %s", text)
	}
}

func TestPageTemplateResolvedFromTemplateDir(t *testing.T) {
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/broadcast.html", "BROADCAST PAGE TEMPLATE")

	admin := core.New()
	page := admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error { return nil }))
	renderer, err := NewRenderer(admin, "/admin", dir)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	html, err := renderer.RenderPage(nil, "", page, "pages/broadcast.html", nil, nil)
	if err != nil {
		t.Fatalf("render page: %v", err)
	}
	if !strings.Contains(html, "BROADCAST PAGE TEMPLATE") {
		t.Fatalf("got %s", html)
	}
}

func TestPageTemplateMissingFromAnyTemplateDirErrors(t *testing.T) {
	admin := core.New()
	page := admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error { return nil }))
	renderer, err := NewRenderer(admin, "/admin", t.TempDir())
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	if _, err := renderer.RenderPage(nil, "", page, "pages/does-not-exist.html", nil, nil); err == nil {
		t.Fatalf("expected an error for a page template with no framework-default fallback")
	}
}

func TestWithStaticDirServesApplicationFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	admin := core.New(core.WithModelAdmins(newTestUserAdmin()))
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin", WithStaticDir(dir)); err != nil {
		t.Fatalf("mount: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/static/custom.css", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); text != "body{}" {
		t.Fatalf("got %q", text)
	}
}
