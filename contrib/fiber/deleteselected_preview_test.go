package fiber

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func noCascade([]any) core.DeletePreview { return core.DeletePreview{} }

func twoUsers(t *testing.T, preview func([]any) core.DeletePreview) (*fiber.App, *previewUserAdmin) {
	t.Helper()
	app, users, _ := newPreviewApp(t, preview)
	users.createUser("a@example.com", true)
	users.createUser("b@example.com", true)
	return app, users
}

func TestDeleteSelectedAsksFirstWhenPreviewing(t *testing.T) {
	app, users := twoUsers(t, noCascade)
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"pks": {"1", "2"}}, nil)
	page := body(t, resp)
	for _, want := range []string{"Delete 2 records?", `name="_confirmed"`, `name="pks" value="1"`, `name="pks" value="2"`, "a@example.com", `id="delete-confirm"`} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if resp.StatusCode != fiber.StatusOK || len(users.store) != 2 {
		t.Errorf("got %d with %d users left; the first POST must delete nothing", resp.StatusCode, len(users.store))
	}
}

func TestConfirmedDeleteSelectedDeletes(t *testing.T) {
	app, users := twoUsers(t, noCascade)
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"pks": {"1", "2"}, "_confirmed": {"1"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || len(users.store) != 0 {
		t.Errorf("got %d with %d users left", resp.StatusCode, len(users.store))
	}
}

func TestConfirmedDeleteSelectedIsRefusedWhenBlocked(t *testing.T) {
	app, users := twoUsers(t, protectedInvoices)
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"pks": {"1"}, "_confirmed": {"1"}}, nil)
	page := body(t, resp)
	if !strings.Contains(page, "This can&#39;t be deleted") || strings.Contains(page, `id="delete-confirm"`) || len(users.store) != 2 {
		t.Errorf("a blocked bulk delete went through or offered to")
	}
}

func TestSelectAllCarriesFiltersAndAFingerprint(t *testing.T) {
	app, users := twoUsers(t, noCascade)
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"_select_all": {"1"}, "search": {"a@"}}, nil)
	page := body(t, resp)
	fp := core.SelectionFingerprint(users, []any{users.store[1]})
	for _, want := range []string{`name="_select_all" value="1"`, `name="search" value="a@"`, `name="_fingerprint" value="` + fp + `"`, "Delete 1 record?"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(page, `name="pks"`) {
		t.Error("select-all must carry the filters, not the pks")
	}
}

func TestChangedSelectionIsShownAgainAndNothingIsDeleted(t *testing.T) {
	app, users := twoUsers(t, noCascade)
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{
		"_select_all": {"1"}, "_confirmed": {"1"}, "_fingerprint": {"stale"},
	}, nil)
	if !strings.Contains(body(t, resp), "The selection changed since you reviewed it.") || len(users.store) != 2 {
		t.Error("a changed selection was deleted without review")
	}
}

func TestConfirmedSelectAllWithTheReviewedFingerprintDeletes(t *testing.T) {
	app, users := twoUsers(t, noCascade)
	fp := core.SelectionFingerprint(users, []any{users.store[1], users.store[2]})
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{
		"_select_all": {"1"}, "_confirmed": {"1"}, "_fingerprint": {fp},
	}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || len(users.store) != 0 {
		t.Errorf("got %d with %d users left", resp.StatusCode, len(users.store))
	}
}

func TestConfirmationReturnsToTheListItCameFrom(t *testing.T) {
	app, _ := twoUsers(t, noCascade)
	page := body(t, doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"pks": {"1"}},
		map[string]string{"Referer": "/admin/users?search=a"}))
	if !strings.Contains(page, `name="_return" value="/admin/users?search=a"`) {
		t.Fatal("the confirmation page lost where it came from")
	}
	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{
		"pks": {"1"}, "_confirmed": {"1"}, "_return": {"/admin/users?search=a"},
	}, map[string]string{"Referer": "/admin/users/actions/delete_selected"})
	if got := resp.Header.Get("Location"); got != "/admin/users?search=a" {
		t.Errorf("Location = %q", got)
	}
}

func TestOverriddenDeleteSelectedStillPreviews(t *testing.T) {
	app, users, _ := newPreviewApp(t, noCascade)
	users.DeclaredActions = []core.Action{core.NewAction(core.DeleteSelectedName, func(ctx context.Context, m core.ModelAdmin, objs []any, p *core.Principal) (string, error) {
		return "", nil
	})}
	users.createUser("a@example.com", true)
	if !strings.Contains(body(t, doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{"pks": {"1"}}, nil)), `name="_confirmed"`) {
		t.Error("an overriding delete_selected skipped the confirmation page")
	}
}

func TestBulkBarMarksThePreviewAndKeepsOtherModals(t *testing.T) {
	app, users, _ := newPreviewApp(t, noCascade)
	users.DeclaredActions = []core.Action{core.NewAction("archive", func(ctx context.Context, m core.ModelAdmin, objs []any, p *core.Principal) (string, error) {
		return "", nil
	}, core.WithActionConfirm("Archive them?"))}
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, "data-preview") || !strings.Contains(page, `data-confirm="Archive them?"`) {
		t.Error("delete_selected is not marked, or another action lost its modal")
	}
	if strings.Contains(page, `data-confirm="Delete the selected records? This cannot be undone."`) {
		t.Error("delete_selected still opens the modal")
	}
}
