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

func TestActionBarSubmitsOnSelectWithNoApplyStep(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	userAdmin.createUser("a@example.com", true)

	text := body(t, doGet(t, app, "/admin/users", nil))
	if strings.Contains(text, `name="action_choice"`) || strings.Contains(text, ">Apply<") {
		t.Fatalf("expected the select+Apply flow to be gone, got %s", text)
	}
	if !strings.Contains(text, `role="listbox"`) || !strings.Contains(text, `role="option"`) {
		t.Fatalf("expected a listbox of actions, got %s", text)
	}
	if !strings.Contains(text, `data-url="/admin/users/actions/deactivate"`) {
		t.Fatalf("expected each action's route on its own item, got %s", text)
	}
}

func TestListViewHidesActionBarWhenNoActionsDeclared(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users", nil)
	if strings.Contains(body(t, resp), `id="bulk-actions-form"`) {
		t.Fatalf("expected no action bar")
	}
}

// Both record pages use ui "page": a width-capped column centered on
// both axes, with the action bar pinned to the bottom so a long record
// scrolls underneath it instead of burying its own buttons.
func TestDetailAndFormPagesShareTheSamePageShell(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	detail := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID), nil))
	edit := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID)+"/edit", nil))
	for _, page := range []string{detail, edit} {
		if !strings.Contains(page, "max-w-xl") {
			t.Error("expected the page to cap its width")
		}
		if !strings.Contains(page, "mx-auto my-auto") {
			t.Error("expected the content centered on both axes")
		}
		if !strings.Contains(page, "sticky bottom-0") {
			t.Error("expected the action bar pinned to the bottom")
		}
	}
}

// The buttons must be outside the scrolling column -- if they drift back
// into it they scroll away on a long record, which is the whole thing
// the sticky bar exists to prevent.
func TestRecordPageActionsSitInTheStickyBar(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	page := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID)+"/edit", nil))
	barAt := strings.Index(page, "sticky bottom-0")
	saveAt := strings.Index(page, "Save and continue editing")
	if barAt == -1 || saveAt < barAt {
		t.Error("expected the Save buttons to render inside the sticky action bar")
	}
	// Outside #resource-form, so they need the form attribute to submit it.
	if !strings.Contains(page, `form="resource-form"`) {
		t.Error("expected the detached submit buttons to name their form")
	}
}

func TestDetailPageButtonsComeAfterTheRecordNotBeforeIt(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	page := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID), nil))
	dlAt := strings.Index(page, "<dl")
	editLinkAt := strings.Index(page, "/edit\"")
	if dlAt == -1 || editLinkAt == -1 || editLinkAt < dlAt {
		t.Errorf("expected the record (<dl>, at %d) to precede the Edit button (at %d)", dlAt, editLinkAt)
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

// A validation error re-renders the whole form wrapper (executeForm runs
// the entire "content" block), so the swap has to replace the wrapper.
// It used to target the inner <form> with outerHTML, which nested a
// fresh wrapper inside the old one on every failed save -- duplicating
// the inline sections and the action bar.
func TestFormErrorSwapTargetsTheWrapperNotTheInnerForm(t *testing.T) {
	app, _ := makeActionApp(t)

	page := body(t, doGet(t, app, "/admin/users/create", nil))
	if !strings.Contains(page, `hx-target="#resource-form-wrapper"`) {
		t.Fatal("swap must target the wrapper, since the error response is the whole wrapper")
	}

	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {""}},
		map[string]string{"HX-Request": "true"})
	errorBody := body(t, resp)
	// The response must be exactly one wrapper -- the element the swap
	// replaces -- so re-rendering it can never nest.
	if n := strings.Count(errorBody, `id="resource-form-wrapper"`); n != 1 {
		t.Errorf("expected exactly 1 wrapper in the error response, got %d", n)
	}
	if n := strings.Count(errorBody, "Save and continue editing"); n != 1 {
		t.Errorf("expected exactly 1 action bar in the error response, got %d", n)
	}
}

// Delete belongs to the edit form only: there is nothing to delete
// while creating, and the detail page deliberately no longer offers it,
// so looking at a record can't put a destructive action one click away.
func TestDeleteButtonOnlyAppearsWhileEditing(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)
	deleteHref := "/admin/users/" + strconv.Itoa(a.ID) + "/delete"

	edit := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID)+"/edit", nil))
	if !strings.Contains(edit, deleteHref) {
		t.Error("expected the edit form to offer Delete")
	}
	detail := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID), nil))
	if strings.Contains(detail, deleteHref) {
		t.Error("expected the detail page not to offer Delete")
	}
	create := body(t, doGet(t, app, "/admin/users/create", nil))
	if strings.Contains(create, "/delete") {
		t.Error("expected the create form not to offer Delete -- there is no record yet")
	}
}

// Delete sits alone on the left, Save/Cancel on the right, so the
// destructive action is never adjacent to the one people click by
// reflex.
func TestDeleteIsSeparatedFromTheSaveButtons(t *testing.T) {
	app, userAdmin := makeActionApp(t)
	a := userAdmin.createUser("a@example.com", true)

	edit := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID)+"/edit", nil))
	deleteAt := strings.Index(edit, "/delete")
	groupAt := strings.Index(edit, "sm:ml-auto")
	saveAt := strings.Index(edit, "Save and continue editing")
	if deleteAt == -1 || groupAt == -1 || saveAt == -1 {
		t.Fatal("expected Delete, the right-hand group, and Save all present")
	}
	if !(deleteAt < groupAt && groupAt < saveAt) {
		t.Errorf("expected Delete (%d) before the right-hand group (%d) wrapping Save (%d)",
			deleteAt, groupAt, saveAt)
	}
}

type denyDeleteAuthorizer struct{}

func (denyDeleteAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "users.delete"
}

func TestDeleteButtonHiddenWhenAuthorizerDeniesIt(t *testing.T) {
	userAdmin := newActionableUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithAuthorizer(denyDeleteAuthorizer{}))
	app := newTestApp(t, admin)
	a := userAdmin.createUser("a@example.com", true)

	edit := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(a.ID)+"/edit", nil))
	if strings.Contains(edit, "/delete") {
		t.Error("expected Delete to be omitted when the authorizer denies it")
	}
}
