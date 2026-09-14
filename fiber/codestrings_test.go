package fiber

import (
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
	flash := setCookies(resp)
	if !strings.Contains(flash, "Utilisateur") || !strings.Contains(flash, "enregistrement cr") {
		t.Errorf("flash cookie %q is not in French", flash)
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
	if flash := setCookies(resp); !strings.Contains(flash, "1 enregistrement supprim") {
		t.Errorf("flash cookie %q", flash)
	}
	resp = doPostForm(t, app, "/admin/users/actions/"+core.DeleteSelectedName, url.Values{}, fr)
	if flash := setCookies(resp); !strings.Contains(flash, "Aucun") {
		t.Errorf("flash cookie %q", flash)
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
