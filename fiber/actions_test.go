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

func deactivateHandler(ctx context.Context, modelAdmin core.ModelAdmin, objects []any, principal *core.Principal) (string, error) {
	for _, obj := range objects {
		obj.(*testUser).IsActive = false
	}
	return "Deactivated", nil
}

func newActionableUserAdmin() *testUserAdmin {
	a := newTestUserAdmin()
	a.DeclaredActions = []core.Action{
		core.NewAction("deactivate", deactivateHandler, core.WithActionConfirm("Deactivate selected users?")),
	}
	return a
}

func makeActionApp(t *testing.T) (*fiber.App, *testUserAdmin) {
	t.Helper()
	userAdmin := newActionableUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin))
	return newTestApp(t, admin), userAdmin
}

func TestBulkActionRunsHandlerOverSelectedObjects(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)
	b := userAdmin.createUser("b@example.com", true)
	c := userAdmin.createUser("c@example.com", true)

	form := url.Values{"pks": {strconv.Itoa(a.ID), strconv.Itoa(b.ID)}}
	resp := doPostForm(t, app, "/admin/users/actions/deactivate", form, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if userAdmin.store[a.ID].IsActive || userAdmin.store[b.ID].IsActive {
		t.Fatalf("expected a and b deactivated")
	}
	if !userAdmin.store[c.ID].IsActive {
		t.Fatalf("expected c untouched")
	}
}

func TestRecordActionFromDetailPageSelectsOneObject(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	resp := doPostForm(t, app, "/admin/users/actions/deactivate", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if userAdmin.store[a.ID].IsActive {
		t.Fatalf("expected a deactivated")
	}
}

func TestBulkActionWithNoSelectionSkipsHandler(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	resp := doPostForm(t, app, "/admin/users/actions/deactivate", url.Values{}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if !userAdmin.store[a.ID].IsActive {
		t.Fatalf("handler should not have run")
	}
}

func TestUnknownActionNameIs404(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	resp := doPostForm(t, app, "/admin/users/actions/nonexistent", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestActionRouteNotRegisteredWhenNoActionsDeclared(t *testing.T) {
	app, _ := makeApp(t)
	resp := doPostForm(t, app, "/admin/users/actions/deactivate", url.Values{"pks": {"1"}}, nil)
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

type denyDeactivateAuthorizer struct{}

func (denyDeactivateAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "users.deactivate"
}

func TestActionRequiresExtraPermissionWhenDeclared(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.DeclaredActions = []core.Action{
		core.NewAction("deactivate", deactivateHandler, core.WithActionPermission("deactivate")),
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithAuthorizer(denyDeactivateAuthorizer{}))
	app := newTestApp(t, admin)
	a := userAdmin.createUser("a@example.com", true)

	resp := doPostForm(t, app, "/admin/users/actions/deactivate", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if !userAdmin.store[a.ID].IsActive {
		t.Fatalf("handler should not have run")
	}
}

func TestListViewShowsActionBarAndCheckboxes(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	userAdmin.createUser("a@example.com", true)

	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, `id="bulk-actions-form"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, "Deactivate") {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, `name="pks"`) {
		t.Fatalf("got %s", text)
	}
}

func TestListViewHidesActionBarWhenNoActionsDeclared(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	if strings.Contains(body(t, resp), `id="bulk-actions-form"`) {
		t.Fatalf("expected no action bar")
	}
}

func TestDetailViewShowsRecordActionButton(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	resp := doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID), nil)
	text := body(t, resp)
	if !strings.Contains(text, "/admin/users/actions/deactivate") {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, "Deactivate") {
		t.Fatalf("got %s", text)
	}
}
