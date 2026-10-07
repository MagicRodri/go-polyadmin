package fiber

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/gofiber/fiber/v2"
)

type refusingUserAdmin struct{ *testUserAdmin }

func refusal() error {
	return fmt.Errorf("save: %w", &core.RecordFormError{Errors: map[string][]string{
		"":      {"That address is reserved."},
		"Email": {"Already taken."},
	}})
}

func (refusingUserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	return nil, refusal()
}

func (refusingUserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	return nil, refusal()
}

func (refusingUserAdmin) Delete(ctx context.Context, obj any) error {
	return &core.RecordFormError{Errors: map[string][]string{"": {"Still referenced by 3 orders."}}}
}

func refusingApp(t *testing.T, opts ...core.Option) (*fiber.App, *testUserAdmin) {
	t.Helper()
	users := newTestUserAdmin()
	users.AllowSaveAs = true
	admin := core.New(append([]core.Option{core.WithModelAdmins(refusingUserAdmin{users})}, opts...)...)
	return newTestApp(t, admin), users
}

func assertRefusedForm(t *testing.T, status int, page string) {
	t.Helper()
	if status != 422 {
		t.Fatalf("status %d", status)
	}
	for _, want := range []string{"That address is reserved.", "Already taken.", `value="kept@x.com"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
}

func TestCreateRefusedByTheModelAdminRerendersTheForm(t *testing.T) {
	app, users := refusingApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"kept@x.com"}, "IsActive": {"true"}}, nil)
	assertRefusedForm(t, resp.StatusCode, readBody(t, resp.Body))
	if len(users.store) != 0 {
		t.Fatal("a record was stored")
	}
}

func TestCreateRefusedRerendersTheFragmentForHTMX(t *testing.T) {
	app, _ := refusingApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"kept@x.com"}, "IsActive": {"true"}}, map[string]string{"HX-Request": "true"})
	page := readBody(t, resp.Body)
	assertRefusedForm(t, resp.StatusCode, page)
	if strings.Contains(page, "<html") {
		t.Error("htmx got a full page")
	}
}

func TestUpdateRefusedByTheModelAdminRerendersTheForm(t *testing.T) {
	app, users := refusingApp(t)
	u := users.createUser("old@x.com", true)
	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/edit", url.Values{"Email": {"kept@x.com"}, "IsActive": {"true"}}, nil)
	assertRefusedForm(t, resp.StatusCode, readBody(t, resp.Body))
	if users.store[u.ID].Email != "old@x.com" {
		t.Fatal("the record changed")
	}
}

func TestSaveAsNewRefusedRerendersTheEditForm(t *testing.T) {
	app, users := refusingApp(t)
	u := users.createUser("old@x.com", true)
	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/edit",
		url.Values{"Email": {"kept@x.com"}, "IsActive": {"true"}, saveAsNewField: {"1"}}, nil)
	page := readBody(t, resp.Body)
	assertRefusedForm(t, resp.StatusCode, page)
	if !strings.Contains(page, fmt.Sprintf("/admin/users/%d/edit", u.ID)) || len(users.store) != 1 || users.store[u.ID].Email != "old@x.com" {
		t.Fatal("save-as-new did not re-render the original record's edit form untouched")
	}
}

type nonFieldValidatingAdmin struct{ *testUserAdmin }

func (a nonFieldValidatingAdmin) Validate(ctx context.Context, data map[string]any) map[string][]string {
	errs := a.testUserAdmin.Validate(ctx, data)
	if errs == nil {
		errs = map[string][]string{}
	}
	errs[""] = append(errs[""], "Signups are closed.")
	return errs
}

func TestValidationNonFieldErrorsRender(t *testing.T) {
	app := newTestApp(t, core.New(core.WithModelAdmins(nonFieldValidatingAdmin{newTestUserAdmin()})))
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"a@x.com"}, "IsActive": {"true"}}, nil)
	if body := readBody(t, resp.Body); resp.StatusCode != 422 || !strings.Contains(body, "Signups are closed.") {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

type deleteAuditLogger struct{ entries []core.AuditEntry }

func (l *deleteAuditLogger) Record(ctx context.Context, e core.AuditEntry) error {
	l.entries = append(l.entries, e)
	return nil
}

func TestDeleteRefusedRedirectsToTheDeletePageWithTheReason(t *testing.T) {
	logger := &deleteAuditLogger{}
	app, users := refusingApp(t, core.WithAuditLogger(logger))
	u := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/delete", url.Values{}, nil)
	if resp.StatusCode != 303 || resp.Header.Get("Location") != fmt.Sprintf("/admin/users/%d/delete", u.ID) {
		t.Fatalf("status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if got := flashText(t, resp); got != "Still referenced by 3 orders." || flashLevels(t, resp) != "error" {
		t.Fatalf("flash %q at %q", got, flashLevels(t, resp))
	}
	if _, ok := users.store[u.ID]; !ok || len(logger.entries) != 0 {
		t.Fatal("the record went away or a delete was audited")
	}
}

func TestHTMXDeleteRefusedRedirectsToTheDeletePage(t *testing.T) {
	app, users := refusingApp(t)
	u := users.createUser("a@x.com", true)
	resp := doDelete(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/delete", map[string]string{"HX-Request": "true"})
	if got := resp.Header.Get("HX-Redirect"); got != fmt.Sprintf("/admin/users/%d/delete", u.ID) {
		t.Fatalf("HX-Redirect %q", got)
	}
	if got := flashText(t, resp); got != "Still referenced by 3 orders." || flashLevels(t, resp) != "error" {
		t.Fatalf("flash %q at %q", got, flashLevels(t, resp))
	}
	if _, ok := users.store[u.ID]; !ok {
		t.Fatal("the row was deleted")
	}
}

func TestTheDeletePageShowsWhyTheDeleteWasRefused(t *testing.T) {
	app, users := refusingApp(t)
	u := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/delete", url.Values{}, nil)
	cookie := ""
	for _, header := range resp.Header.Values("Set-Cookie") {
		if rest, ok := strings.CutPrefix(header, flashCookieName+"="); ok {
			cookie, _, _ = strings.Cut(rest, ";")
		}
	}
	page := readBody(t, doGet(t, app, resp.Header.Get("Location"), map[string]string{"Cookie": flashCookieName + "=" + cookie}).Body)
	if !strings.Contains(page, "Still referenced by 3 orders.") {
		t.Fatal("the delete page does not show the refusal")
	}
}
