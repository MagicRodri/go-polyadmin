package fiber

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/gofiber/fiber/v2"
)

func actionErrorApp(t *testing.T, logger core.AuditLogger, actions ...core.Action) (*fiber.App, *testUserAdmin) {
	t.Helper()
	users := newTestUserAdmin()
	users.DeclaredActions = actions
	opts := []core.Option{core.WithModelAdmins(users)}
	if logger != nil {
		opts = append(opts, core.WithAuditLogger(logger))
	}
	return newTestApp(t, core.New(opts...)), users
}

func failingAction(name, level string, done func([]any) []any) core.Action {
	return core.NewAction(name, func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (string, error) {
		return "", &core.ActionError{Message: "Archive is offline.", Level: level, Done: done(objects)}
	})
}

func runAction(t *testing.T, app *fiber.App, name string, pks ...int) *http.Response {
	t.Helper()
	form := url.Values{"_confirmed": {"1"}}
	for _, pk := range pks {
		form.Add("pks", strconv.Itoa(pk))
	}
	return doPostForm(t, app, "/admin/users/actions/"+name, form, nil)
}

func TestActionErrorFlashesAtItsLevel(t *testing.T) {
	for _, tc := range []struct{ level, want string }{{"", "error"}, {"error", "error"}, {"warning", "warning"}} {
		app, users := actionErrorApp(t, nil, failingAction("archive", tc.level, func([]any) []any { return nil }))
		u := users.createUser("a@x.com", true)
		resp := runAction(t, app, "archive", u.ID)
		if resp.StatusCode != fiber.StatusSeeOther {
			t.Fatalf("level %q: status %d", tc.level, resp.StatusCode)
		}
		if flashText(t, resp) != "Archive is offline." || flashLevels(t, resp) != tc.want {
			t.Fatalf("level %q: flash %q at %q", tc.level, flashText(t, resp), flashLevels(t, resp))
		}
	}
}

func TestActionErrorAuditsOnlyDone(t *testing.T) {
	logger := &bulkAuditLogger{}
	app, users := actionErrorApp(t, logger, failingAction("archive", "", func(objects []any) []any { return objects[:1] }))
	a := users.createUser("a@x.com", true)
	b := users.createUser("b@x.com", true)
	runAction(t, app, "archive", a.ID, b.ID)
	if len(logger.entries) != 1 || logger.entries[0].Action != "archive" {
		t.Fatalf("entries %+v", logger.entries)
	}
}

func TestActionErrorWithNoDoneAuditsNothing(t *testing.T) {
	logger := &bulkAuditLogger{}
	app, users := actionErrorApp(t, logger, failingAction("archive", "", func([]any) []any { return nil }))
	a := users.createUser("a@x.com", true)
	runAction(t, app, "archive", a.ID)
	if len(logger.entries) != 0 {
		t.Fatalf("entries %+v", logger.entries)
	}
}

type failOnSecondUpdate struct {
	*testUserAdmin
	calls int
}

func (f *failOnSecondUpdate) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	f.calls++
	if f.calls == 2 {
		return nil, &core.RecordFormError{Errors: map[string][]string{"": {"Locked."}}}
	}
	return f.testUserAdmin.Update(ctx, obj, data)
}

func TestBulkEditPartialFailureAuditsDoneAsUpdates(t *testing.T) {
	logger := &bulkAuditLogger{}
	users := newTestUserAdmin()
	users.BulkEditFieldNames = []string{"IsActive"}
	failing := &failOnSecondUpdate{testUserAdmin: users}
	app := newTestApp(t, core.New(core.WithModelAdmins(failing), core.WithAuditLogger(logger)))
	a := users.createUser("a@x.com", true)
	b := users.createUser("b@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/bulk_edit",
		url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}, "_confirmed": {"1"}, "_change_IsActive": {"1"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || flashLevels(t, resp) != "error" {
		t.Fatalf("status %d levels %q", resp.StatusCode, flashLevels(t, resp))
	}
	if got := flashText(t, resp); got != "Updated 1 of 2, then failed: Locked." {
		t.Fatalf("flash %q", got)
	}
	if len(logger.entries) != 1 || logger.entries[0].Action != core.AuditUpdate {
		t.Fatalf("entries %+v", logger.entries)
	}
}

func TestFormActionReturningActionErrorRedirects(t *testing.T) {
	action := core.NewAction("note", nil,
		core.WithActionForm(core.NewField("Note", core.FieldTypeString)),
		core.WithActionFormHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, data map[string]any, p *core.Principal) (core.ActionResult, error) {
			return core.ActionResult{}, &core.ActionError{Message: "Archive is offline.", Level: "warning"}
		}))
	app, users := actionErrorApp(t, nil, action)
	a := users.createUser("a@x.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/note", url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Note": {"x"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || flashLevels(t, resp) != "warning" {
		t.Fatalf("status %d levels %q", resp.StatusCode, flashLevels(t, resp))
	}
}
