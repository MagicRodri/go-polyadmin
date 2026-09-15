package fiber

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// A host catalog, so the tests do not depend on the framework catalogs'
// contents (Task 13 fills them).
var hostFrench = fstest.MapFS{"fr.json": &fstest.MapFile{Data: []byte(`{"Hello": "Bonjour", "Broadcast Message": "Message diffusé"}`)}}

func helloPageAdmin(t *testing.T, opts ...core.Option) (*core.Admin, string) {
	t.Helper()
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/hello.html", `<p id="greeting">{{t "Hello"}}</p><p id="lang">{{locale}}</p>`)
	admin := core.New(append([]core.Option{core.WithCatalogs(hostFrench)}, opts...)...)
	admin.Route("/hello", PageHandler(func(pc *PageContext) error {
		return pc.Render("pages/hello.html", nil)
	}), core.WithPageLabel("Broadcast Message"))
	return admin, dir
}

func TestPageRendersInTheAcceptLanguageLocale(t *testing.T) {
	admin, dir := helloPageAdmin(t)
	app := mountPageApp(t, admin, dir)

	page := body(t, doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "fr-CA,fr;q=0.9"}))
	if !strings.Contains(page, `<p id="greeting">Bonjour</p>`) {
		t.Errorf("expected the French string, got %s", page)
	}
	if !strings.Contains(page, `<html lang="fr"`) {
		t.Error(`expected <html lang="fr">`)
	}

	english := body(t, doGet(t, app, "/admin/hello", nil))
	if !strings.Contains(english, `<p id="greeting">Hello</p>`) || !strings.Contains(english, `<html lang="en"`) {
		t.Error("without a preference the page is English")
	}
}

func TestLocaleCookieBeatsAcceptLanguage(t *testing.T) {
	admin, dir := helloPageAdmin(t)
	app := mountPageApp(t, admin, dir)
	page := body(t, doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "fr", "Cookie": "admin_locale=en"}))
	if !strings.Contains(page, "<p id=\"greeting\">Hello</p>") {
		t.Errorf("the cookie must win, got %s", page)
	}
}

// Ruling R7: with the switcher off there is no way to change the cookie
// from the admin, so a leftover one (it lives a year) must not beat the
// resolver or the browser's language.
func TestSwitcherOffIgnoresTheLocaleCookie(t *testing.T) {
	forced := core.WithLocaleResolver(func(any, *core.Principal) string { return "fr" })
	admin, dir := helloPageAdmin(t, core.WithoutLocaleSwitcher(), forced)
	page := body(t, doGet(t, mountPageApp(t, admin, dir), "/admin/hello", map[string]string{"Cookie": "admin_locale=ru"}))
	if !strings.Contains(page, `<html lang="fr"`) || !strings.Contains(page, "Bonjour") {
		t.Errorf("the resolver's fr must beat a stale ru cookie when the switcher is off, got %s", page)
	}

	admin, dir = helloPageAdmin(t, core.WithoutLocaleSwitcher())
	page = body(t, doGet(t, mountPageApp(t, admin, dir), "/admin/hello", map[string]string{"Cookie": "admin_locale=ru", "Accept-Language": "fr"}))
	if !strings.Contains(page, `<html lang="fr"`) {
		t.Errorf("Accept-Language must beat a stale cookie when the switcher is off, got %s", page)
	}
}

type countingAuthenticator struct{ calls int }

func (a *countingAuthenticator) Authenticate(request any) *core.Principal {
	a.calls++
	return &core.Principal{ID: "u", Extra: map[string]any{"locale": "fr"}}
}

func TestResolverSeesThePrincipalAndAuthenticationRunsOnce(t *testing.T) {
	auth := &countingAuthenticator{}
	admin, dir := helloPageAdmin(t,
		core.WithAuthenticator(auth),
		core.WithLocaleResolver(func(request any, p *core.Principal) string {
			if p == nil {
				return ""
			}
			locale, _ := p.Extra["locale"].(string)
			return locale
		}),
	)
	app := mountPageApp(t, admin, dir)

	page := body(t, doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "en"}))
	if !strings.Contains(page, "Bonjour") {
		t.Errorf("the resolver must beat the header, got %s", page)
	}
	if auth.calls != 1 {
		t.Errorf("Authenticate ran %d times in one request, want 1", auth.calls)
	}
}

func TestHostStringsTranslateThroughT(t *testing.T) {
	admin, dir := helloPageAdmin(t)
	app := mountPageApp(t, admin, dir)
	// The page label reaches the sidebar through {{t .Label}} in Task 7;
	// here, {{t}} with a non-literal argument is exercised directly.
	writeOverrideTemplate(t, dir, "pages/hello.html", `{{$label := "Broadcast Message"}}<p id="label">{{t $label}}</p>`)
	page := body(t, doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "fr"}))
	if !strings.Contains(page, `<p id="label">Message diffusé</p>`) {
		t.Errorf("got %s", page)
	}
}

func TestTjsEmitsAJSONStringLiteralInAnAttribute(t *testing.T) {
	apostrophe := fstest.MapFS{"fr.json": &fstest.MapFile{Data: []byte(`{"Dark mode": "Mode d'affichage \"sombre\""}`)}}
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/hello.html", `<span id="x" x-text="dark ? {{tjs "Dark mode"}} : ''"></span>`)
	admin := core.New(core.WithCatalogs(apostrophe))
	admin.Route("/hello", PageHandler(func(pc *PageContext) error { return pc.Render("pages/hello.html", nil) }))
	app := mountPageApp(t, admin, dir)

	page := body(t, doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "fr"}))
	// Attribute-escaped JSON: the browser decodes &#34;/&#39; back before
	// Alpine evaluates, leaving a valid JS string literal.
	want := `x-text="dark ? &#34;Mode d&#39;affichage \&#34;sombre\&#34;&#34; : ''"`
	if !strings.Contains(page, want) {
		t.Errorf("got %s", page)
	}
}

func TestErrorPagesUseTheRequestLocale(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users/999", map[string]string{"Accept-Language": "ru"}))
	if !strings.Contains(page, `<html lang="ru"`) {
		t.Errorf("expected the 404 page in the request locale, got %s", page)
	}
}

func TestPageContextExposesTheLocale(t *testing.T) {
	var seen string
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/hello.html", "x")
	admin := core.New()
	admin.Route("/hello", PageHandler(func(pc *PageContext) error {
		seen = pc.Locale()
		return pc.Render("pages/hello.html", nil)
	}))
	app := mountPageApp(t, admin, dir)
	doGet(t, app, "/admin/hello", map[string]string{"Accept-Language": "fr"})
	if seen != "fr" {
		t.Errorf("PageContext.Locale() = %q", seen)
	}
}

func TestModelAdminContextCarriesTheLocale(t *testing.T) {
	ua := newTestUserAdmin()
	ua.createUser("a@example.com", true)
	var seen string
	ua.onGetObject = func(locale string) { seen = locale }
	app := newTestApp(t, core.New(core.WithModelAdmins(ua)))
	doGet(t, app, "/admin/users/1", map[string]string{"Accept-Language": "ru"})
	if seen != "ru" {
		t.Errorf("core.Locale(ctx) inside GetObject = %q", seen)
	}
}

// Static files are not admin pages: resolving their locale would run the
// LocaleResolver, and so authenticate, on every CSS and JS request.
func TestStaticFilesSkipLocaleResolution(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "custom.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	auth := &countingAuthenticator{}
	resolved := 0
	admin := core.New(core.WithAuthenticator(auth), core.WithLocaleResolver(func(any, *core.Principal) string {
		resolved++
		return ""
	}))
	app := fiber.New()
	if err := Mount(app.Group("/admin"), admin, "/admin", WithStaticDir(dir)); err != nil {
		t.Fatal(err)
	}
	resp := doGet(t, app, "/admin/static/custom.css", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if resolved != 0 || auth.calls != 0 {
		t.Errorf("a static file ran the resolver %d and the authenticator %d times", resolved, auth.calls)
	}
}
