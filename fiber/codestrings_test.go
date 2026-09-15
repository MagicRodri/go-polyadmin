package fiber

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

var frenchCodeStrings = fstest.MapFS{"fr.json": &fstest.MapFile{Data: []byte(`{
	"User": "Utilisateur",
	"Email": "Courriel",
	"%s created.": "%s : enregistrement créé.",
	"%s is required.": "%s est obligatoire.",
	"Not found": "Introuvable",
	"No items selected.": "Aucun élément sélectionné.",
	"Deleted %d record.": {"one": "%d enregistrement supprimé.", "other": "%d enregistrements supprimés."}
}`)}}

func frenchApp(t *testing.T) (*fiber.App, *testUserAdmin) {
	t.Helper()
	ua := newTestUserAdmin()
	return newTestApp(t, core.New(core.WithModelAdmins(ua), core.WithCatalogs(frenchCodeStrings))), ua
}

var fr = map[string]string{"Accept-Language": "fr"}

// setCookies joins every Set-Cookie header: the CSRF cookie comes first,
// so Header.Get alone would miss the flash.
func setCookies(resp *http.Response) string {
	return strings.Join(resp.Header.Values("Set-Cookie"), "\n")
}

func TestFlashIsTranslated(t *testing.T) {
	app, _ := frenchApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"a@example.com"}}, fr)
	flash := flashText(t, resp)
	if !strings.Contains(flash, "Utilisateur") || !strings.Contains(flash, "enregistrement cr") {
		t.Errorf("flash %q is not in French", flash)
	}
}

func TestRequiredErrorIsTranslated(t *testing.T) {
	app, _ := frenchApp(t)
	page := body(t, doPostForm(t, app, "/admin/users/create", url.Values{"Email": {""}}, fr))
	if !strings.Contains(page, "Courriel est obligatoire.") {
		t.Errorf("expected the French required error, got %s", page)
	}
}

func TestErrorPageIsTranslated(t *testing.T) {
	app, _ := frenchApp(t)
	if page := body(t, doGet(t, app, "/admin/users/999", fr)); !strings.Contains(page, "Introuvable") {
		t.Errorf("got %s", page)
	}
}

func TestBuiltInActionMessagesAreTranslated(t *testing.T) {
	app, ua := frenchApp(t)
	ua.createUser("a@example.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/"+core.DeleteSelectedName, url.Values{"pks": {"1"}}, fr)
	if flash := flashText(t, resp); !strings.Contains(flash, "1 enregistrement supprim") {
		t.Errorf("flash %q", flash)
	}
	resp = doPostForm(t, app, "/admin/users/actions/"+core.DeleteSelectedName, url.Values{}, fr)
	if flash := flashText(t, resp); !strings.Contains(flash, "Aucun") {
		t.Errorf("flash %q", flash)
	}
}

// TestDeleteSelectedFlashIsNotDoubleWrappedInPseudoLocale guards R5: the
// built-in delete action already translates its own result with
// core.TN, and handleAction's tr(c, message) translates it a second
// time so a host's own static result gets its turn too (see the
// comment at that call site). Composing the two passes must stay a
// no-op -- under the pseudo-locale, a second wrap would nest the
// brackets, which the sweep's allow-list can't catch since it strips
// bracket runs rather than counting them.
func TestDeleteSelectedFlashIsNotDoubleWrappedInPseudoLocale(t *testing.T) {
	ua := newTestUserAdmin()
	ua.createUser("a@example.com", true)
	app := newTestApp(t, core.New(core.WithModelAdmins(ua), core.WithPseudoLocale()))
	resp := doPseudoPostForm(t, app, "/admin/users/actions/"+core.DeleteSelectedName, url.Values{"pks": {"1"}})

	got := flashText(t, resp)
	if got == "" {
		t.Fatalf("no flash text in %q", setCookies(resp))
	}
	want := fmt.Sprintf(core.Pseudo("Deleted %d record."), 1)
	if got != want {
		t.Errorf("flash text = %q, want %q (single pseudo-wrapped)", got, want)
	}
}

func TestExportHeadersAreTranslated(t *testing.T) {
	app, ua := frenchApp(t)
	ua.createUser("a@example.com", true)
	resp := doGet(t, app, "/admin/users/export/csv", fr)
	raw, _ := io.ReadAll(resp.Body)
	if first := strings.SplitN(string(raw), "\n", 2)[0]; !strings.Contains(first, "Courriel") {
		t.Errorf("CSV header %q", first)
	}
}

// The framework's own fallback strings -- a root page's default label,
// AllowAllAuthenticator's default display name, the empty admin's notice
// -- are framework strings like any other and come out translated.
func TestFrameworkDefaultsAreTranslated(t *testing.T) {
	ru := map[string]string{"Cookie": localeCookieName + "=ru"}

	admin := core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)))
	admin.Route("/", PageHandler(func(pc *PageContext) error { return nil }))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", ru))
	for _, want := range []string{`class="truncate">Страница</span>`, "Аноним"} {
		if !strings.Contains(page, want) {
			t.Errorf("the Russian page lacks %q", want)
		}
	}

	empty := body(t, doGet(t, newTestApp(t, core.New()), "/admin", ru))
	if !strings.Contains(empty, "Нет зарегистрированных ресурсов.") {
		t.Errorf("the empty admin's notice is not translated: %q", empty)
	}
}
