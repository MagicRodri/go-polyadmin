package fiber

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// recordingLogger is the write side only -- deliberately not an
// AuditReader, so the tests can tell the two capabilities apart.
type recordingLogger struct {
	entries []core.AuditEntry
	err     error
}

func (l *recordingLogger) Record(ctx context.Context, entry core.AuditEntry) error {
	l.entries = append(l.entries, entry)
	return l.err
}

// readableLogger adds the read side.
type readableLogger struct{ recordingLogger }

func (l *readableLogger) History(ctx context.Context, resource string, pk any, limit int) ([]core.AuditEntry, error) {
	var out []core.AuditEntry
	for _, e := range l.entries {
		if e.Resource == resource {
			out = append(out, e)
		}
	}
	return out, nil
}

func auditApp(t *testing.T, logger core.AuditLogger) (*fiber.App, *testUserAdmin) {
	t.Helper()
	userAdmin := newActionableUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithAuditLogger(logger))
	return newTestApp(t, admin), userAdmin
}

func TestCreateUpdateAndDeleteAreRecorded(t *testing.T) {
	logger := &recordingLogger{}
	app, userAdmin := auditApp(t, logger)

	doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"new@example.com"}}, nil)
	u := userAdmin.createUser("edit-me@example.com", true)
	id := strconv.Itoa(u.ID)
	doPostForm(t, app, "/admin/users/"+id+"/edit", url.Values{"Email": {"edited@example.com"}}, nil)
	doDelete(t, app, "/admin/users/"+id+"/delete", nil)

	var actions []string
	for _, e := range logger.entries {
		actions = append(actions, e.Action)
	}
	if got := strings.Join(actions, ","); got != "create,update,delete" {
		t.Errorf("recorded %q, want create,update,delete", got)
	}
	for _, e := range logger.entries {
		if e.Resource != "users" {
			t.Errorf("entry has resource %q", e.Resource)
		}
		if e.At.IsZero() {
			t.Error("entry has no timestamp")
		}
	}
}

// The label is captured at write time because the record may be gone by
// the time anyone reads the log -- which is exactly the case for a
// delete.
func TestDeleteEntryKeepsTheRecordsLabel(t *testing.T) {
	logger := &recordingLogger{}
	app, userAdmin := auditApp(t, logger)
	u := userAdmin.createUser("goodbye@example.com", true)

	doDelete(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/delete", nil)

	if len(logger.entries) != 1 {
		t.Fatalf("expected one entry, got %d", len(logger.entries))
	}
	if !strings.Contains(logger.entries[0].ObjectLabel, "goodbye@example.com") {
		t.Errorf("label = %q, want the deleted record's own label", logger.entries[0].ObjectLabel)
	}
}

// One entry per record, not one per action run.
func TestBulkActionRecordsOneEntryPerRecord(t *testing.T) {
	logger := &recordingLogger{}
	app, userAdmin := auditApp(t, logger)
	a := userAdmin.createUser("a@example.com", true)
	b := userAdmin.createUser("b@example.com", true)

	doPostForm(t, app, "/admin/users/actions/deactivate",
		url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}}, nil)

	if len(logger.entries) != 2 {
		t.Fatalf("expected one entry per record, got %d", len(logger.entries))
	}
	for _, e := range logger.entries {
		if e.Action != "deactivate" {
			t.Errorf("action = %q, want the action's own name", e.Action)
		}
	}
}

// A failed change must not be recorded as one.
func TestNothingIsRecordedWhenTheChangeIsRejected(t *testing.T) {
	logger := &recordingLogger{}
	app, _ := auditApp(t, logger)

	// Email is required, so this never reaches Create.
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {""}}, nil)
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("expected the save to be rejected, got %d", resp.StatusCode)
	}
	if len(logger.entries) != 0 {
		t.Errorf("a rejected save was recorded: %+v", logger.entries)
	}
}

// A logger that errors must not fail the request: the change already
// happened, and showing an error next to it would be a lie.
func TestALoggerErrorDoesNotFailTheChange(t *testing.T) {
	logger := &recordingLogger{err: errors.New("log is down")}
	app, _ := auditApp(t, logger)

	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"a@example.com"}}, nil)
	if resp.StatusCode >= 400 {
		t.Errorf("got %d -- a logging failure must not fail the write", resp.StatusCode)
	}
}

func TestNoLoggerConfiguredRecordsNothingAndStillWorks(t *testing.T) {
	app, _ := makeActionApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"a@example.com"}}, nil)
	if resp.StatusCode >= 400 {
		t.Errorf("got %d with no audit logger configured", resp.StatusCode)
	}
}

// The read side is a separate capability: recording alone shows nothing.
func TestHistoryPanelAppearsOnlyForAReadableLogger(t *testing.T) {
	writeOnly := &recordingLogger{}
	app, userAdmin := auditApp(t, writeOnly)
	u := userAdmin.createUser("a@example.com", true)
	doPostForm(t, app, "/admin/users/"+strconv.Itoa(u.ID)+"/edit", url.Values{"Email": {"b@example.com"}}, nil)

	page := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(u.ID), nil))
	if strings.Contains(page, ">History<") {
		t.Error("a write-only logger must not render a History panel")
	}

	readable := &readableLogger{}
	app2, userAdmin2 := auditApp(t, readable)
	u2 := userAdmin2.createUser("a@example.com", true)
	doPostForm(t, app2, "/admin/users/"+strconv.Itoa(u2.ID)+"/edit", url.Values{"Email": {"b@example.com"}}, nil)

	page2 := body(t, doGet(t, app2, "/admin/users/"+strconv.Itoa(u2.ID), nil))
	if !strings.Contains(page2, ">History<") {
		t.Error("a readable logger should render the History panel")
	}
	if !strings.Contains(page2, "update") {
		t.Error("expected the recorded change to be listed")
	}
}
