package fiber

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func TestDeleteSelectedIsOfferedWithoutDeclaringIt(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, "Delete selected") {
		t.Error("the built-in bulk delete is not offered")
	}
}

func TestDeleteSelectedActuallyDeletes(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.store[1] = &testUser{ID: 1, Email: "a@example.com"}
	userAdmin.store[2] = &testUser{ID: 2, Email: "b@example.com"}
	userAdmin.store[3] = &testUser{ID: 3, Email: "c@example.com"}

	resp := doPostForm(t, app, "/admin/users/actions/delete_selected", url.Values{
		"pks": {"1", "2"},
	}, nil)
	if resp.StatusCode != fiber.StatusSeeOther && resp.StatusCode != fiber.StatusOK {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if _, still := userAdmin.store[1]; still {
		t.Error("record 1 survived")
	}
	if _, still := userAdmin.store[2]; still {
		t.Error("record 2 survived")
	}
	// The unticked one must not be touched.
	if _, kept := userAdmin.store[3]; !kept {
		t.Error("record 3 was deleted but was never selected")
	}
}

func TestDeleteSelectedCarriesAConfirmation(t *testing.T) {
	action := core.NewDeleteSelectedAction()
	if action.Confirm == "" {
		t.Error("the built-in bulk delete has no confirmation prompt")
	}
}

func TestDeleteSelectedRequiresTheDeletePermission(t *testing.T) {
	if got := core.NewDeleteSelectedAction().Permission; got != "delete" {
		t.Errorf("permission = %q, want %q", got, "delete")
	}
}

func TestDeleteSelectedIsAbsentWhenDeleteIsDisabled(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.DisableDelete = true
	app := newTestApp(t, core.New(core.WithModelAdmins(userAdmin)))
	if strings.Contains(body(t, doGet(t, app, "/admin/users", nil)), "Delete selected") {
		t.Error("bulk delete offered on an admin that cannot delete")
	}
}

func TestDeleteSelectedCanBeOptedOut(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.DisableDeleteSelected = true
	app := newTestApp(t, core.New(core.WithModelAdmins(userAdmin)))
	if strings.Contains(body(t, doGet(t, app, "/admin/users", nil)), "Delete selected") {
		t.Error("DisableDeleteSelected was ignored")
	}
}

func TestDeclaringDeleteSelectedReplacesTheBuiltIn(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.DeclaredActions = []core.Action{
		core.NewAction(core.DeleteSelectedName, func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (string, error) {
			return "mine ran", nil
		}, core.WithActionLabel("Delete selected")),
	}
	if got := len(userAdmin.Actions()); got != 1 {
		t.Fatalf("got %d actions, want the declared one only", got)
	}
	if userAdmin.Actions()[0].Label != "Delete selected" {
		t.Error("the built-in displaced the declared action")
	}
}
