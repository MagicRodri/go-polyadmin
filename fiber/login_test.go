package fiber

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// fakeLoginBackend is a LoginBackend whose session is a single cookie
// holding the principal's ID -- enough to exercise the flow end to end
// without pulling a real session implementation into the library's
// tests. It doubles as the Authenticator, which is the pairing the
// docs describe: one writes the session, the other reads it.
type fakeLoginBackend struct {
	password string
	// begins/ends count calls, so a test can tell "the page rendered"
	// from "a session was actually established".
	begins int
	ends   int
	// beginErr simulates a session store that is down.
	beginErr error
}

const fakeSessionCookie = "test_session"

func (b *fakeLoginBackend) VerifyCredentials(request any, identifier, password string) *core.Principal {
	if (identifier != "demo@example.com" && identifier != "demo") || password != b.password {
		return nil
	}
	return &core.Principal{ID: "demo", DisplayName: "Demo Admin", IsSuperuser: true}
}

func (b *fakeLoginBackend) BeginSession(request any, principal *core.Principal) error {
	if b.beginErr != nil {
		return b.beginErr
	}
	b.begins++
	request.(*fiber.Ctx).Cookie(&fiber.Cookie{Name: fakeSessionCookie, Value: "demo", Path: "/"})
	return nil
}

func (b *fakeLoginBackend) EndSession(request any) error {
	b.ends++
	request.(*fiber.Ctx).ClearCookie(fakeSessionCookie)
	return nil
}

func (b *fakeLoginBackend) Authenticate(request any) *core.Principal {
	if request.(*fiber.Ctx).Cookies(fakeSessionCookie) == "" {
		return nil
	}
	return &core.Principal{ID: "demo", DisplayName: "Demo Admin", IsSuperuser: true}
}

func newLoginApp(t *testing.T) (*fiber.App, *fakeLoginBackend) {
	t.Helper()
	backend := &fakeLoginBackend{password: "correct horse"}
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(backend),
		core.WithLoginBackend(backend),
	)
	return newTestApp(t, admin), backend
}

func TestUnauthenticatedRequestRedirectsToLogin(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d, want a redirect to the login page", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/admin/login?") {
		t.Fatalf("Location = %q, want the login page", location)
	}
	// The whole point of the redirect: it has to come back afterwards.
	if got := parseNext(t, location); got != "/admin/users" {
		t.Errorf("next = %q, want the page they asked for", got)
	}
}

func TestLoginRedirectPreservesTheQueryString(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doGet(t, app, "/admin/users?page=3&sort=Email", nil)
	if got := parseNext(t, resp.Header.Get("Location")); got != "/admin/users?page=3&sort=Email" {
		t.Errorf("next = %q, want the query string kept", got)
	}
}

func TestUnauthenticatedHTMXRequestGetsHXRedirect(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doGet(t, app, "/admin/users", map[string]string{"HX-Request": "true"})
	if got := resp.Header.Get("HX-Redirect"); !strings.HasPrefix(got, "/admin/login") {
		t.Errorf("HX-Redirect = %q, want the login page", got)
	}
}

func TestWithoutALoginBackendUnauthenticatedIsStill401(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.DenyAllAuthenticator{}),
	)
	resp := doGet(t, newTestApp(t, admin), "/admin/users", nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
}

func TestWithoutALoginBackendTheLoginRouteIsNotMounted(t *testing.T) {
	app, _ := makeApp(t)
	if resp := doGet(t, app, "/admin/login", nil); resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d, want 404 -- the login page must not mount without a backend", resp.StatusCode)
	}
}

func TestLoginPageIsPubliclyReachable(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doGet(t, app, "/admin/login", nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	page := body(t, resp)
	for _, want := range []string{
		`name="identifier"`,
		`type="password"`,
		`name="_csrf"`, // no-JS submit still has to carry a token
		"Welcome back",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("login page is missing %q", want)
		}
	}
}

func TestLoginPageRendersWithoutTheAdminShell(t *testing.T) {
	app, _ := newLoginApp(t)
	page := body(t, doGet(t, app, "/admin/login", nil))
	for _, unwanted := range []string{"ui/sidebar", "sidebarOpen", "Breadcrumb", "breadcrumb"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("login page rendered %q -- it must not use the admin layout", unwanted)
		}
	}
	// But it must still carry the theme, or signing in flashes a light
	// page at someone who chose dark.
	if !strings.Contains(page, "polyadmin-theme") {
		t.Error("login page is missing the theme block")
	}
}

func TestLoginPageOmitsControlsWithNoRouteBehindThem(t *testing.T) {
	app, _ := newLoginApp(t)
	page := body(t, doGet(t, app, "/admin/login", nil))
	for _, unwanted := range []string{"Forgot your password", "Sign up", "Login with Google", "Or continue with"} {
		if strings.Contains(page, unwanted) {
			t.Errorf("login page renders %q, which has no route behind it", unwanted)
		}
	}
}

func TestLoginIdentifierFieldAcceptsPlainText(t *testing.T) {
	app, _ := newLoginApp(t)
	page := body(t, doGet(t, app, "/admin/login", nil))
	if strings.Contains(page, `type="email"`) {
		t.Error("identifier input is type=email, which rejects a plain username before it reaches the server")
	}
	if !strings.Contains(page, "Username or email") {
		t.Error("login page label should invite a username, not just an email")
	}
}

func TestLoginAcceptsAUsernameNotOnlyAnEmail(t *testing.T) {
	app, backend := newLoginApp(t)
	resp := doPostForm(t, app, "/admin/login?next=%2Fadmin%2Fusers", url.Values{
		"identifier": {"demo"},
		"password":   {"correct horse"},
	}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d, want 303 -- a username should sign in the same as the email", resp.StatusCode)
	}
	if backend.begins != 1 {
		t.Errorf("BeginSession called %d times, want exactly 1", backend.begins)
	}
}

func TestValidCredentialsBeginASessionAndReturnToNext(t *testing.T) {
	app, backend := newLoginApp(t)
	resp := doPostForm(t, app, "/admin/login?next=%2Fadmin%2Fusers", url.Values{
		"identifier": {"demo@example.com"},
		"password":   {"correct horse"},
	}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d, want 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "/admin/users" {
		t.Errorf("Location = %q, want the page they were headed for", got)
	}
	if backend.begins != 1 {
		t.Errorf("BeginSession called %d times, want exactly 1", backend.begins)
	}
}

func TestInvalidCredentialsDoNotBeginASession(t *testing.T) {
	app, backend := newLoginApp(t)
	resp := doPostForm(t, app, "/admin/login", url.Values{
		"identifier": {"demo@example.com"},
		"password":   {"wrong"},
	}, nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
	if backend.begins != 0 {
		t.Fatalf("BeginSession was called %d times on a failed sign-in", backend.begins)
	}
	if !strings.Contains(body(t, resp), "don&#39;t match an account") {
		t.Error("expected the failure message on the redisplayed form")
	}
}

func TestFailedSignInEchoesTheIdentifierBack(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doPostForm(t, app, "/admin/login", url.Values{
		"identifier": {"demo@example.com"},
		"password":   {"wrong"},
	}, nil)
	if !strings.Contains(body(t, resp), `value="demo@example.com"`) {
		t.Error("expected the submitted email to be redisplayed")
	}
}

func TestUnknownUserAndWrongPasswordAreIndistinguishable(t *testing.T) {
	app, _ := newLoginApp(t)
	wrongPassword := body(t, doPostForm(t, app, "/admin/login", url.Values{
		"identifier": {"demo@example.com"}, "password": {"wrong"},
	}, nil))
	noSuchUser := body(t, doPostForm(t, app, "/admin/login", url.Values{
		"identifier": {"nobody@example.com"}, "password": {"wrong"},
	}, nil))
	// Compare the rendered alert, not the whole page -- the echoed
	// identifier differs by construction.
	if extractAlert(t, wrongPassword) != extractAlert(t, noSuchUser) {
		t.Error("the two failures render different messages, which enumerates accounts")
	}
}

func TestSessionFailureDoesNotSignAnyoneIn(t *testing.T) {
	app, backend := newLoginApp(t)
	backend.beginErr = errStoreDown
	resp := doPostForm(t, app, "/admin/login", url.Values{
		"identifier": {"demo@example.com"},
		"password":   {"correct horse"},
	}, nil)
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("got %d, want 500", resp.StatusCode)
	}
	if strings.Contains(resp.Header.Get("Location"), "/admin/users") {
		t.Error("redirected into the admin despite the session failing")
	}
}

func TestSignInRefusesToRedirectOffSite(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doPostForm(t, app, "/admin/login?next=https%3A%2F%2Fevil.example", url.Values{
		"identifier": {"demo@example.com"},
		"password":   {"correct horse"},
	}, nil)
	if got := resp.Header.Get("Location"); got != "/admin" {
		t.Errorf("Location = %q, want the admin's own base path", got)
	}
}

func TestLoginPageRedirectsAnAlreadySignedInVisitor(t *testing.T) {
	app, _ := newLoginApp(t)
	resp := doGet(t, app, "/admin/login", map[string]string{"Cookie": fakeSessionCookie + "=demo"})
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d, want a redirect away from the form", resp.StatusCode)
	}
}

func TestSidebarOffersSignOutWhenALoginBackendIsConfigured(t *testing.T) {
	app, _ := newLoginApp(t)
	page := body(t, doGet(t, app, "/admin/users", map[string]string{"Cookie": fakeSessionCookie + "=demo"}))
	if !strings.Contains(page, "Sign out") {
		t.Fatal("no sign-out control in the sidebar")
	}
	// A form, not a link: see the template's note on GET logouts.
	if !strings.Contains(page, `<form method="post" action="/admin/logout">`) {
		t.Error("sign-out must POST, not link")
	}
}

// TestTheUserMenuHoldsActionsOnly: the trigger the menu opens from sits
// directly beside it and already shows who you are, so an identity
// header inside the menu told the reader nothing new.
func TestTheUserMenuHoldsActionsOnly(t *testing.T) {
	app, _ := newLoginApp(t)
	page := body(t, doGet(t, app, "/admin/users", map[string]string{"Cookie": fakeSessionCookie + "=demo"}))
	footer := sidebarFooter(t, page)

	if !strings.Contains(footer, "Sign out") {
		t.Fatal("no sign-out in the footer; the assertions below would be vacuous")
	}
	if got := strings.Count(footer, "Demo Admin"); got != 1 {
		t.Errorf("the signed-in name appears %d times in the footer, want 1", got)
	}
	if got := strings.Count(footer, "Superuser"); got != 1 {
		t.Errorf("the signed-in role appears %d times in the footer, want 1", got)
	}
	label, err := uiClasses("dropdown", "label")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.Contains(footer, label) {
		t.Error("the user menu still carries an identity header")
	}
}

// TestWithNothingToSignOutOfTheFooterOpensNothing: with no login
// backend the menu would hold no items at all, and a trigger that opens
// an empty box is a dead end.
func TestWithNothingToSignOutOfTheFooterOpensNothing(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.NewAllowAllAuthenticator(&core.Principal{ID: "demo", DisplayName: "Demo"})),
	)
	footer := sidebarFooter(t, body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil)))
	if !strings.Contains(footer, "Demo") {
		t.Fatal("the footer does not name the principal; the assertion below would be vacuous")
	}
	if strings.Contains(footer, `aria-haspopup="menu"`) {
		t.Error("the footer still opens a menu with nothing in it")
	}
}

// sidebarFooter returns the sidebar's footer region, bounded at the
// rail that follows it.
func sidebarFooter(t *testing.T, page string) string {
	t.Helper()
	classes, err := uiClasses("sidebar", "footer")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	start := strings.Index(page, classes)
	if start < 0 {
		t.Fatal("no sidebar footer on the page")
	}
	footer := page[start:]
	end := strings.Index(footer, `aria-label="Toggle sidebar"`)
	if end < 0 {
		t.Fatal("no sidebar rail after the footer")
	}
	return footer[:end]
}

func TestSidebarOmitsSignOutWithoutALoginBackend(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.NewAllowAllAuthenticator(&core.Principal{ID: "demo", DisplayName: "Demo"})),
	)
	if strings.Contains(body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil)), "Sign out") {
		t.Error("sign-out offered with no logout route behind it")
	}
}

func TestLogoutEndsTheSessionAndSaysSo(t *testing.T) {
	app, backend := newLoginApp(t)
	// Built by hand rather than via doPostForm: this request needs both
	// the session cookie and the CSRF cookie, and they share one header.
	resp := postSignedOut(t, app, fakeSessionCookie+"=demo")
	if backend.ends != 1 {
		t.Fatalf("EndSession called %d times, want exactly 1", backend.ends)
	}
	if got := resp.Header.Get("Location"); !strings.HasPrefix(got, "/admin/login") {
		t.Fatalf("Location = %q, want the login page", got)
	}
	// The session must actually be cleared, not merely redirected away
	// from.
	var cleared bool
	for _, header := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(header, fakeSessionCookie+"=") {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logout sent no Set-Cookie for the session")
	}
	page := body(t, doGet(t, app, "/admin/login?signedout=1", nil))
	if !strings.Contains(page, "signed out") {
		t.Error("expected the login page to confirm the sign-out")
	}
}

func TestLogoutRejectsGET(t *testing.T) {
	app, backend := newLoginApp(t)
	if resp := doGet(t, app, "/admin/logout", nil); resp.StatusCode != fiber.StatusMethodNotAllowed && resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d, want GET to be refused", resp.StatusCode)
	}
	if backend.ends != 0 {
		t.Error("a GET ended the session")
	}
}

var errStoreDown = &storeDownError{}

type storeDownError struct{}

func (*storeDownError) Error() string { return "session store unavailable" }

// postSignedOut POSTs to the logout route carrying both the caller's
// cookies and a matching CSRF pair -- doPostForm sets the Cookie header
// itself, so a test that also needs a session cookie has to merge them.
func postSignedOut(t *testing.T, app *fiber.App, cookies string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", "/admin/logout", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := core.NewCSRFToken()
	req.Header.Set("Cookie", cookies+"; "+core.CSRFCookieName+"="+token)
	req.Header.Set(core.CSRFHeaderName, token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func parseNext(t *testing.T, location string) string {
	t.Helper()
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse %q: %v", location, err)
	}
	return parsed.Query().Get(core.NextQueryParam)
}

// extractAlert pulls the rendered alert's text out of the page, so two
// failure renderings can be compared without the echoed identifier
// (which differs by construction) making them trivially unequal.
func extractAlert(t *testing.T, page string) string {
	t.Helper()
	const marker = `role="alert"`
	start := strings.Index(page, marker)
	if start < 0 {
		t.Fatal("no alert on the page")
	}
	rest := page[start:]
	end := strings.Index(rest, "</div>")
	if end < 0 {
		t.Fatal("unterminated alert")
	}
	return rest[:end]
}
