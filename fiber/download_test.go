package fiber

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func makeDownloadApp(t *testing.T) (*fiber.App, *testUserAdmin) {
	t.Helper()
	users := newTestUserAdmin()
	emails := func(objects []any, sep string) string {
		var parts []string
		for _, obj := range objects {
			parts = append(parts, obj.(*testUser).Email)
		}
		return strings.Join(parts, sep)
	}
	users.DeclaredActions = []core.Action{
		core.NewAction("emails", nil, core.WithActionDownloadHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (core.ActionResult, error) {
			return core.ActionResult{Download: &core.Download{Filename: "emails.csv", ContentType: "text/csv", Content: []byte(emails(objects, "\n"))}}, nil
		})),
		core.NewAction("streamed", nil, core.WithActionDownloadHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (core.ActionResult, error) {
			return core.ActionResult{Download: &core.Download{Filename: "сотрудники.txt", ContentType: "text/plain", Stream: strings.NewReader(emails(objects, "\n") + "\n")}}, nil
		})),
		core.NewAction("joined", nil,
			core.WithActionForm(core.NewField("Sep", core.FieldTypeString, core.WithRequired())),
			core.WithActionFormHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, data map[string]any, p *core.Principal) (core.ActionResult, error) {
				return core.ActionResult{Download: &core.Download{Filename: "joined.txt", Content: []byte(emails(objects, data["Sep"].(string)))}}, nil
			})),
	}
	return newTestApp(t, core.New(core.WithModelAdmins(users))), users
}

func TestDownloadActionAnswersWithTheFile(t *testing.T) {
	app, users := makeDownloadApp(t)
	a := users.createUser("a@x.com", true)
	b := users.createUser("b@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/emails", url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}}, nil)
	body := readBody(t, resp.Body)
	if resp.StatusCode != fiber.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") ||
		!strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="emails.csv"`) {
		t.Fatalf("status %d headers %v", resp.StatusCode, resp.Header)
	}
	if body != "a@x.com\nb@x.com" {
		t.Fatalf("body %q", body)
	}
}

func TestStreamedDownloadEncodesNonASCIIName(t *testing.T) {
	app, users := makeDownloadApp(t)
	a := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/streamed", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	want := "filename*=UTF-8''%D1%81%D0%BE%D1%82%D1%80%D1%83%D0%B4%D0%BD%D0%B8%D0%BA%D0%B8.txt"
	if !strings.Contains(resp.Header.Get("Content-Disposition"), want) || readBody(t, resp.Body) != "a@x.com\n" {
		t.Fatalf("headers %v", resp.Header)
	}
}

func TestFormActionCanAnswerWithADownload(t *testing.T) {
	app, users := makeDownloadApp(t)
	a := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/joined", url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Sep": {";"}}, nil)
	if readBody(t, resp.Body) != "a@x.com" || resp.Header.Get("Content-Type") != "application/octet-stream" {
		t.Fatalf("headers %v", resp.Header)
	}
}

func TestContentDispositionFallback(t *testing.T) {
	if !strings.HasPrefix(contentDisposition(`a"b.csv`), `attachment; filename="ab.csv"`) {
		t.Fatal(contentDisposition(`a"b.csv`))
	}
	if !strings.HasPrefix(contentDisposition("файл.csv"), `attachment; filename=".csv"`) {
		t.Fatal(contentDisposition("файл.csv"))
	}
}

func TestDownloadWithNilContentSendsAnEmptyFile(t *testing.T) {
	users := newTestUserAdmin()
	users.DeclaredActions = []core.Action{
		core.NewAction("empty", nil, core.WithActionDownloadHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (core.ActionResult, error) {
			return core.ActionResult{Download: &core.Download{Filename: "empty.csv", Content: []byte(nil), ContentType: "text/csv"}}, nil
		})),
	}
	app := newTestApp(t, core.New(core.WithModelAdmins(users)))
	a := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/empty", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if resp.StatusCode != fiber.StatusOK || readBody(t, resp.Body) != "" {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
