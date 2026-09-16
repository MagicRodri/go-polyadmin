package fiber

// The msgids carry a straight apostrophe and html/template escapes it, as
// login_test.go's "don&#39;t match an account" does; the browser shows "'".
import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// previewUserAdmin is the users admin plus a DeletePreview each test sets.
type previewUserAdmin struct {
	*testUserAdmin
	preview func(objects []any) core.DeletePreview
}

func (a *previewUserAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
	return a.preview(objects), nil
}

// newPreviewApp mounts "users" (with the preview) and "notes", a second
// registered resource for the preview to point at.
func newPreviewApp(t *testing.T, preview func([]any) core.DeletePreview, opts ...core.Option) (*fiber.App, *previewUserAdmin, *testUserAdmin) {
	t.Helper()
	users := &previewUserAdmin{testUserAdmin: newTestUserAdmin(), preview: preview}
	notes := newTestUserAdmin()
	notes.ModelName = "Note"
	notes.SlugOverride = "notes"
	admin := core.New(append([]core.Option{core.WithModelAdmins(users, notes)}, opts...)...)
	return newTestApp(t, admin), users, notes
}

func cascadeToNotes(notes *testUserAdmin, total int) func([]any) core.DeletePreview {
	return func([]any) core.DeletePreview {
		objs := []any{notes.store[1], notes.store[2]}
		return core.DeletePreview{Cascades: []core.DeleteGroup{{Resource: "notes", Objects: objs, Total: total}}}
	}
}

func protectedInvoices([]any) core.DeletePreview {
	return core.DeletePreview{Protected: []core.DeleteGroup{{Label: "Invoices", Objects: []any{"INV-1"}}}}
}

type hideNoteAuthorizer struct{ email string }

func (a hideNoteAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	u, ok := resource.(*testUser)
	return !(permission == "notes.view" && ok && u.Email == a.email)
}

type denyNotesDeleteAuthorizer struct{}

func (denyNotesDeleteAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "notes.delete"
}

func TestDeletePageNamesTheRecord(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)
	page := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/delete", nil))
	for _, want := range []string{"Delete «john@example.com»?", "This action cannot be undone.", `id="delete-confirm"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(page, "This will also delete") {
		t.Error("a ModelAdmin without the capability shows a cascade")
	}
}

func TestDeletePageListsTheCascade(t *testing.T) {
	var notes *testUserAdmin
	app, users, notes := newPreviewApp(t, func(o []any) core.DeletePreview { return cascadeToNotes(notes, 5)(o) })
	users.createUser("a@example.com", true)
	notes.createUser("n1@example.com", true)
	notes.createUser("n2@example.com", true)
	page := body(t, doGet(t, app, "/admin/users/1/delete", nil))
	for _, want := range []string{"This will also delete", "Note (5)", `href="/admin/notes/1"`, "n2@example.com", "…and 3 more", `id="delete-confirm"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestDeletePageCountsRecordsTheUserCannotView(t *testing.T) {
	var notes *testUserAdmin
	app, users, notes := newPreviewApp(t, func(o []any) core.DeletePreview { return cascadeToNotes(notes, 2)(o) },
		core.WithAuthorizer(hideNoteAuthorizer{email: "n2@example.com"}))
	users.createUser("a@example.com", true)
	notes.createUser("n1@example.com", true)
	notes.createUser("n2@example.com", true)
	page := body(t, doGet(t, app, "/admin/users/1/delete", nil))
	if !strings.Contains(page, "1 you can&#39;t view") || strings.Contains(page, "n2@example.com") {
		t.Errorf("hidden record leaked or was not counted")
	}
}

func TestProtectedRecordsBlockTheDeletePage(t *testing.T) {
	app, users, _ := newPreviewApp(t, protectedInvoices)
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, app, "/admin/users/1/delete", nil))
	for _, want := range []string{"This can&#39;t be deleted", "These records must be removed first:", "Invoices (1)", "INV-1"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(page, `id="delete-confirm"`) {
		t.Error("a blocked delete still offers the Delete button")
	}
}

func TestCascadeIntoAForbiddenTypeBlocks(t *testing.T) {
	var notes *testUserAdmin
	app, users, notes := newPreviewApp(t, func(o []any) core.DeletePreview { return cascadeToNotes(notes, 2)(o) },
		core.WithAuthorizer(denyNotesDeleteAuthorizer{}))
	users.createUser("a@example.com", true)
	notes.createUser("n1@example.com", true)
	notes.createUser("n2@example.com", true)
	page := body(t, doGet(t, app, "/admin/users/1/delete", nil))
	if !strings.Contains(page, "Your account doesn&#39;t have permission to delete: Note") {
		t.Error("the denied type is not named")
	}
}

func TestBlockedDeletePostIsRefused(t *testing.T) {
	app, users, _ := newPreviewApp(t, protectedInvoices)
	users.createUser("a@example.com", true)
	resp := doPostForm(t, app, "/admin/users/1/delete", url.Values{}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || resp.Header.Get("Location") != "/admin/users/1/delete" {
		t.Fatalf("got %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if len(users.store) != 1 {
		t.Error("a blocked record was deleted")
	}
}

func TestUnblockedPreviewStillDeletes(t *testing.T) {
	var notes *testUserAdmin
	app, users, notes := newPreviewApp(t, func(o []any) core.DeletePreview { return cascadeToNotes(notes, 2)(o) })
	users.createUser("a@example.com", true)
	notes.createUser("n1@example.com", true)
	notes.createUser("n2@example.com", true)
	resp := doPostForm(t, app, "/admin/users/1/delete", url.Values{}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || len(users.store) != 0 {
		t.Fatalf("got %d, %d users left", resp.StatusCode, len(users.store))
	}
}

func TestBlockedHTMXDeleteRedirectsToTheDeletePage(t *testing.T) {
	app, users, _ := newPreviewApp(t, protectedInvoices)
	users.createUser("a@example.com", true)
	resp := doDelete(t, app, "/admin/users/1/delete", map[string]string{"HX-Request": "true"})
	if got := resp.Header.Get("HX-Redirect"); got != "/admin/users/1/delete" {
		t.Errorf("HX-Redirect = %q", got)
	}
	if len(users.store) != 1 {
		t.Error("a blocked record was deleted")
	}
}

func TestListRowDeleteLinksToThePageOnlyWhenPreviewing(t *testing.T) {
	app, users, _ := newPreviewApp(t, protectedInvoices)
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, `href="/admin/users/1/delete"`) || strings.Contains(page, `hx-delete="/admin/users/1/delete"`) {
		t.Error("a previewing list still deletes rows in place")
	}
	plain, plainUsers := makeApp(t)
	plainUsers.createUser("a@example.com", true)
	if !strings.Contains(body(t, doGet(t, plain, "/admin/users", nil)), `hx-delete="/admin/users/1/delete"`) {
		t.Error("a list without the capability lost its in-place delete")
	}
}

// previewInlineUserAdmin makes the inline child admin a previewer.
type previewInlineUserAdmin struct{ *inlineTestUserAdmin }

func (a *previewInlineUserAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
	return protectedInvoices(objects), nil
}

func TestBlockedInlineRemoveKeepsTheRowAndSaysWhy(t *testing.T) {
	orgAdmin := newInlineTestOrgAdmin(core.InlineLayoutTabular)
	userAdmin := newInlineTestUserAdmin(orgAdmin)
	admin := core.New(core.WithModelAdmins(orgAdmin, &previewInlineUserAdmin{userAdmin}))
	app := newTestApp(t, admin)
	seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	resp := doDelete(t, app, "/admin/organizations/1/inlines/users/1", map[string]string{"HX-Request": "true"})
	page := body(t, resp)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(page, "This can&#39;t be deleted") || !strings.Contains(page, "INV-1") {
		t.Errorf("got %d: %s", resp.StatusCode, page)
	}
	if len(userAdmin.store) != 1 {
		t.Error("a blocked inline child was deleted")
	}
}
