package fiber

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func switcherApp(t *testing.T, opts ...core.Option) *fiber.App {
	t.Helper()
	ua := newTestUserAdmin()
	ua.createUser("a@example.com", true)
	return newTestApp(t, core.New(append([]core.Option{core.WithModelAdmins(ua)}, opts...)...))
}

func TestLocaleSwitchSetsTheCookieAndRedirectsBack(t *testing.T) {
	app := switcherApp(t)
	resp := doPostForm(t, app, "/admin/locale", url.Values{"locale": {"fr-CA"}}, map[string]string{"Referer": "http://example.com/admin/users?page=2"})
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/admin/users?page=2" {
		t.Fatalf("got %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	cookie := resp.Header.Get("Set-Cookie")
	for _, want := range []string{"admin_locale=fr", "path=/admin", "max-age=31536000", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(strings.ToLower(cookie), strings.ToLower(want)) {
			t.Errorf("Set-Cookie %q lacks %q", cookie, want)
		}
	}
}

func TestLocaleSwitchIgnoresAnUnsupportedLocale(t *testing.T) {
	app := switcherApp(t)
	resp := doPostForm(t, app, "/admin/locale", url.Values{"locale": {"de"}}, nil)
	if resp.StatusCode != 303 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if strings.Contains(resp.Header.Get("Set-Cookie"), "admin_locale") {
		t.Error("an unsupported locale must not be stored")
	}
}

func TestLocaleSwitchRefusesAnOffsiteReferer(t *testing.T) {
	app := switcherApp(t)
	resp := doPostForm(t, app, "/admin/locale", url.Values{"locale": {"fr"}}, map[string]string{"Referer": "https://evil.example/phish"})
	if got := resp.Header.Get("Location"); got != "/admin" {
		t.Errorf("Location = %q, want the admin root", got)
	}
}

func TestLocaleSwitchRequiresCSRF(t *testing.T) {
	app := switcherApp(t)
	req := httptest.NewRequest("POST", "/admin/locale", strings.NewReader("locale=fr"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 403 {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
}

func TestSwitcherRendersInTheHeader(t *testing.T) {
	page := body(t, doGet(t, switcherApp(t), "/admin/users", nil))
	for _, want := range []string{`action="/admin/locale"`, `value="fr"`, "Français", "Русский", `aria-checked="true"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the header switcher lacks %q", want)
		}
	}
}

func TestSwitcherRendersOnTheLoginPage(t *testing.T) {
	backend := &fakeLoginBackend{password: "x"}
	app := switcherApp(t, core.WithAuthenticator(backend), core.WithLoginBackend(backend))
	if page := body(t, doGet(t, app, "/admin/login", nil)); !strings.Contains(page, `action="/admin/locale"`) {
		t.Error("the login page must offer the switcher")
	}
}

func TestNoSwitcherWithOneLocale(t *testing.T) {
	app := switcherApp(t, core.WithLocales("en"))
	if page := body(t, doGet(t, app, "/admin/users", nil)); strings.Contains(page, `action="/admin/locale"`) {
		t.Error("one locale: no switcher")
	}
	if resp := doPostForm(t, app, "/admin/locale", url.Values{"locale": {"en"}}, nil); resp.StatusCode != 404 {
		t.Errorf("one locale: no route, got %d", resp.StatusCode)
	}
}

func TestNoSwitcherWhenDisabled(t *testing.T) {
	app := switcherApp(t, core.WithoutLocaleSwitcher())
	if page := body(t, doGet(t, app, "/admin/users", nil)); strings.Contains(page, `action="/admin/locale"`) {
		t.Error("disabled: no switcher")
	}
}
