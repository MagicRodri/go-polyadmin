package fiber

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// The helpers below deliberately do NOT attach a CSRF token -- unlike
// doPostForm/doDelete, which do. These are for the tests that assert a
// request without a valid token is rejected.

func rawPostForm(path string, form url.Values) *http.Request {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func rawDelete(path string) *http.Request {
	return httptest.NewRequest("DELETE", path, nil)
}

func send(t *testing.T, app *fiber.App, req *http.Request) *http.Response {
	t.Helper()
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

// csrfCookie returns the admin_csrf value a response handed out.
func csrfCookie(t *testing.T, resp *http.Response) string {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == core.CSRFCookieName {
			return c.Value
		}
	}
	return ""
}

func TestGETMintsATokenCookie(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	got := csrfCookie(t, resp)
	if len(got) != 43 {
		t.Errorf("expected a 43-char token cookie, got %q", got)
	}
}

func TestTokenCookieIsHttpOnly(t *testing.T) {
	// The token is rendered into a meta tag for scripts, which is what
	// lets the cookie stay HttpOnly -- so this is a real guarantee, not
	// an accident.
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	for _, c := range resp.Cookies() {
		if c.Name == core.CSRFCookieName && !c.HttpOnly {
			t.Error("the CSRF cookie must be HttpOnly")
		}
	}
}

func TestUnsafeRequestWithoutATokenIsRejected(t *testing.T) {
	app, _ := makeApp(t)
	resp := send(t, app, rawPostForm("/admin/users/create", url.Values{"Email": {"a@example.com"}}))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
}

func TestUnsafeRequestWithAMismatchedTokenIsRejected(t *testing.T) {
	app, _ := makeApp(t)
	req := rawPostForm("/admin/users/create", url.Values{"Email": {"a@example.com"}})
	req.Header.Set("Cookie", core.CSRFCookieName+"="+core.NewCSRFToken())
	req.Header.Set(core.CSRFHeaderName, core.NewCSRFToken())
	if resp := send(t, app, req); resp.StatusCode != http.StatusForbidden {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
}

func TestUnsafeRequestAcceptsTheTokenInTheFormField(t *testing.T) {
	app, _ := makeApp(t)
	token := core.NewCSRFToken()
	req := rawPostForm("/admin/users/create",
		url.Values{"Email": {"a@example.com"}, core.CSRFFieldName: {token}})
	req.Header.Set("Cookie", core.CSRFCookieName+"="+token)
	if resp := send(t, app, req); resp.StatusCode == http.StatusForbidden {
		t.Error("a matching _csrf field should be accepted -- this is the no-JS path")
	}
}

func TestBodylessDeleteAcceptsTheTokenInTheHeader(t *testing.T) {
	app, userAdmin := makeApp(t)
	u := userAdmin.createUser("a@example.com", true)
	token := core.NewCSRFToken()
	req := rawDelete("/admin/users/" + strconv.Itoa(u.ID) + "/delete")
	req.Header.Set("Cookie", core.CSRFCookieName+"="+token)
	req.Header.Set(core.CSRFHeaderName, token)
	if resp := send(t, app, req); resp.StatusCode == http.StatusForbidden {
		t.Error("a matching header should be accepted -- a bodyless DELETE has no field to carry one")
	}
}

func TestBodylessDeleteWithoutTheHeaderIsRejected(t *testing.T) {
	app, userAdmin := makeApp(t)
	u := userAdmin.createUser("a@example.com", true)
	resp := send(t, app, rawDelete("/admin/users/"+strconv.Itoa(u.ID)+"/delete"))
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
}

func TestClickjackingHeadersAreSet(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := resp.Header.Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Errorf("CSP = %q, want frame-ancestors 'none'", got)
	}
}

// makeActionApp, not makeApp: the list page's bulk-actions form and the
// detail page's record-action forms are both inside {{if .Actions}}, so
// an admin with no declared actions renders neither, and the assertions
// below would pass or fail for the wrong reason.
func TestPagesCarryTheTokenForFormsAndHtmx(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	u := userAdmin.createUser("a@example.com", true)
	id := strconv.Itoa(u.ID)

	for _, path := range []string{
		"/admin/users",        // bulk-actions form
		"/admin/users/create", // resource form
		"/admin/users/" + id,  // record actions
		"/admin/users/" + id + "/edit",
		"/admin/users/" + id + "/delete",
	} {
		resp := doGet(t, app, path, nil)
		// Compared against the cookie, not merely present: an empty
		// token still renders both the tag and the field, so a
		// presence-only assertion would pass even if nothing was
		// threaded through at all.
		token := csrfCookie(t, resp)
		page := body(t, resp)
		if !strings.Contains(page, `<meta name="csrf-token" content="`+token+`">`) {
			t.Errorf("%s: no csrf-token meta tag carrying the cookie's token", path)
		}
		if !strings.Contains(page, `<input type="hidden" name="_csrf" value="`+token+`">`) {
			t.Errorf("%s: no hidden _csrf field carrying the cookie's token", path)
		}
	}
}

func TestHtmxRequestsGetTheTokenHeader(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, "htmx:configRequest") {
		t.Error("expected the listener that attaches X-CSRF-Token to htmx requests")
	}
	if !strings.Contains(page, core.CSRFHeaderName) {
		t.Error("expected the header name in the listener")
	}
}

func TestCSRFCanBeDisabled(t *testing.T) {
	userAdmin := newTestUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithCSRFDisabled())
	app := newTestApp(t, admin)

	resp := send(t, app, rawPostForm("/admin/users/create", url.Values{"Email": {"a@example.com"}}))
	if resp.StatusCode == http.StatusForbidden {
		t.Error("verification should be off when WithCSRFDisabled is set")
	}
	// The cookie is still minted, so templates and custom pages are
	// unchanged by the opt-out.
	if csrfCookie(t, resp) == "" {
		t.Error("the cookie should still be minted when verification is off")
	}
	// And the frame headers are not part of the opt-out.
	if resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Error("clickjacking headers must not be disabled by WithCSRFDisabled")
	}
}
