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

type bulkAuditLogger struct{ entries []core.AuditEntry }

func (l *bulkAuditLogger) Record(ctx context.Context, e core.AuditEntry) error {
	l.entries = append(l.entries, e)
	return nil
}

func makeBulkApp(t *testing.T, opts ...core.Option) (*fiber.App, *testUserAdmin) {
	t.Helper()
	users := newTestUserAdmin()
	users.BulkEditFieldNames = []string{"Email", "IsActive"}
	return newTestApp(t, core.New(append([]core.Option{core.WithModelAdmins(users)}, opts...)...)), users
}

func postBulk(t *testing.T, app *fiber.App, form url.Values) (int, string) {
	t.Helper()
	resp := doPostForm(t, app, "/admin/users/actions/bulk_edit", form, nil)
	return resp.StatusCode, readBody(t, resp.Body)
}

func TestBulkEditFormHasChangeBoxes(t *testing.T) {
	app, users := makeBulkApp(t)
	a := users.createUser("a@x.com", true)
	status, body := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID)}})
	if status != fiber.StatusOK || !strings.Contains(body, `name="_change_Email"`) || !strings.Contains(body, `name="_change_IsActive"`) {
		t.Fatalf("status %d", status)
	}
}

func TestBulkEditAppliesOnlyTickedFields(t *testing.T) {
	app, users := makeBulkApp(t)
	a := users.createUser("a@x.com", true)
	b := users.createUser("b@x.com", true)
	status, _ := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}, "_confirmed": {"1"},
		"_change_IsActive": {"1"}, "Email": {"hijack@x.com"}})
	if status != fiber.StatusSeeOther {
		t.Fatalf("status %d", status)
	}
	if users.store[a.ID].IsActive || users.store[b.ID].IsActive || users.store[a.ID].Email != "a@x.com" {
		t.Fatalf("a=%+v b=%+v", users.store[a.ID], users.store[b.ID])
	}
}

func TestBulkEditNothingTicked(t *testing.T) {
	app, users := makeBulkApp(t)
	a := users.createUser("a@x.com", true)
	status, body := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}})
	if status != fiber.StatusUnprocessableEntity || !strings.Contains(body, "Choose at least one field to change.") {
		t.Fatalf("status %d", status)
	}
}

func TestBulkEditValidatesTickedFields(t *testing.T) {
	app, users := makeBulkApp(t)
	a := users.createUser("a@x.com", true)
	status, _ := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "_change_Email": {"1"}, "Email": {""}})
	if status != fiber.StatusUnprocessableEntity || users.store[a.ID].Email != "a@x.com" {
		t.Fatalf("status %d", status)
	}
}

type denyUpdateOnTwo struct{}

func (denyUpdateOnTwo) Can(p *core.Principal, permission string, resource any) bool {
	if u, ok := resource.(*testUser); ok && strings.HasSuffix(permission, ".update") {
		return u.ID != 2
	}
	return true
}

func TestBulkEditBlockedObjectAppliesNothing(t *testing.T) {
	app, users := makeBulkApp(t, core.WithAuthorizer(denyUpdateOnTwo{}))
	a := users.createUser("a@x.com", true)
	b := users.createUser("b@x.com", true)
	status, body := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}, "_confirmed": {"1"}, "_change_IsActive": {"1"}})
	if status != fiber.StatusUnprocessableEntity || !strings.Contains(body, `id="action-form-blocked"`) {
		t.Fatalf("status %d", status)
	}
	if !users.store[a.ID].IsActive || !users.store[b.ID].IsActive {
		t.Fatal("partial update applied")
	}
}

func TestBulkEditAuditsUpdates(t *testing.T) {
	logger := &bulkAuditLogger{}
	app, users := makeBulkApp(t, core.WithAuditLogger(logger))
	a := users.createUser("a@x.com", true)
	postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "_change_IsActive": {"1"}})
	if len(logger.entries) != 1 || logger.entries[0].Action != core.AuditUpdate {
		t.Fatalf("entries %+v", logger.entries)
	}
}

type strictUserAdmin struct{ *testUserAdmin }

func (s strictUserAdmin) Validate(ctx context.Context, data map[string]any) map[string][]string {
	errs := s.testUserAdmin.Validate(ctx, data)
	if active, ok := data["IsActive"].(bool); ok && !active {
		errs["IsActive"] = append(errs["IsActive"], "Users cannot be deactivated in bulk.")
	}
	return errs
}

func TestBulkEditRunsTheModelAdminsOwnValidate(t *testing.T) {
	users := newTestUserAdmin()
	users.BulkEditFieldNames = []string{"Email", "IsActive"}
	app := newTestApp(t, core.New(core.WithModelAdmins(strictUserAdmin{users})))
	a := users.createUser("a@x.com", true)
	status, body := postBulk(t, app, url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "_change_IsActive": {"1"}})
	if status != fiber.StatusUnprocessableEntity || !strings.Contains(body, "Users cannot be deactivated in bulk.") || !users.store[a.ID].IsActive {
		t.Fatalf("status %d active %v", status, users.store[a.ID].IsActive)
	}
}
