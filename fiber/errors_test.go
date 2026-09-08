package fiber

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func TestNotFoundRendersAPageNotBareText(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users/999999", nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d, want 404", resp.StatusCode)
	}
	page := body(t, resp)
	for _, want := range []string{
		"<!doctype html>",
		"polyadmin-theme", // the theme block, so dark mode holds
		"404",
		"Back to the admin",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("404 page is missing %q", want)
		}
	}
	if strings.TrimSpace(page) == "Not found" {
		t.Error("still the bare text body")
	}
}

func TestForbiddenRendersAPage(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.NewAllowAllAuthenticator(&core.Principal{ID: "u", DisplayName: "U"})),
		core.WithAuthorizer(core.DenyAllAuthorizer{}),
	)
	resp := doGet(t, newTestApp(t, admin), "/admin/users", nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d, want 403", resp.StatusCode)
	}
	page := body(t, resp)
	if !strings.Contains(page, "<!doctype html>") || !strings.Contains(page, "403") {
		t.Error("403 did not render as a page")
	}
	// Signed in, so there is somewhere to go back to.
	if !strings.Contains(page, "Back to the admin") {
		t.Error("403 page offers no way back")
	}
}

func TestUnauthenticatedPageOffersNoHomeLink(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.DenyAllAuthenticator{}),
	)
	resp := doGet(t, newTestApp(t, admin), "/admin/users", nil)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("got %d, want 401", resp.StatusCode)
	}
	page := body(t, resp)
	if !strings.Contains(page, "Sign-in required") {
		t.Error("401 did not render the error page")
	}
	if strings.Contains(page, "Back to the admin") {
		t.Error("401 offers a link the visitor cannot follow either")
	}
}

func TestHTMXFailureReturnsAFragmentNotAPage(t *testing.T) {
	admin := core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(core.NewAllowAllAuthenticator(&core.Principal{ID: "u"})),
		core.WithAuthorizer(core.DenyAllAuthorizer{}),
	)
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", map[string]string{"HX-Request": "true"}))
	if strings.Contains(page, "<!doctype html>") {
		t.Error("htmx failure returned a whole document")
	}
	if !strings.Contains(page, `role="alert"`) {
		t.Errorf("expected the alert fragment, got %q", page)
	}
}

func TestCSRFRejectionRendersAPage(t *testing.T) {
	app, _ := makeApp(t)
	// rawPostForm deliberately attaches no token -- see csrf_test.go.
	resp := send(t, app, rawPostForm("/admin/users/create", nil))
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d, want 403", resp.StatusCode)
	}
	page := body(t, resp)
	if !strings.Contains(page, "Security check failed") {
		t.Errorf("expected the CSRF failure page, got %q", truncate(page))
	}
	if !strings.Contains(page, "Reload and try again") {
		t.Error("CSRF page does not tell the user what to do")
	}
}

func truncate(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
