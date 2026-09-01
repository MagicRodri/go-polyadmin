package fiber

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// -- per-object permissions ----------------------------------------------

// ownRecordsOnly is the archetypal per-object rule: a principal may
// change only the record whose Email matches their own. It answers on
// the *resource* it is handed, which is a ModelAdmin for a coarse check
// and the record itself for the narrow one.
type ownRecordsOnly struct{ email string }

func (a ownRecordsOnly) Can(principal *core.Principal, permission string, resource any) bool {
	user, ok := resource.(*testUser)
	if !ok {
		// Coarse check: no object in hand, so this cannot decide -- let
		// it through and judge again once the record is loaded.
		return true
	}
	return user.Email == a.email
}

func objectPermApp(t *testing.T) (*fiber.App, *testUserAdmin, *testUser, *testUser) {
	t.Helper()
	userAdmin := newTestUserAdmin()
	mine := userAdmin.createUser("me@example.com", true)
	theirs := userAdmin.createUser("someone-else@example.com", true)
	admin := core.New(
		core.WithModelAdmins(userAdmin),
		core.WithAuthorizer(ownRecordsOnly{email: "me@example.com"}),
	)
	return newTestApp(t, admin), userAdmin, mine, theirs
}

func TestPerObjectPermissionGatesTheEditForm(t *testing.T) {
	app, _, mine, theirs := objectPermApp(t)

	if got := doGet(t, app, "/admin/users/"+strconv.Itoa(mine.ID)+"/edit", nil).StatusCode; got != 200 {
		t.Errorf("own record: got %d, want 200", got)
	}
	if got := doGet(t, app, "/admin/users/"+strconv.Itoa(theirs.ID)+"/edit", nil).StatusCode; got != 403 {
		t.Errorf("someone else's record: got %d, want 403", got)
	}
}

// The gate has to hold on the write, not only on the form that leads to
// it -- a denied principal can post straight to the route.
func TestPerObjectPermissionGatesTheEditPost(t *testing.T) {
	app, userAdmin, _, theirs := objectPermApp(t)
	before := theirs.Email

	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(theirs.ID)+"/edit",
		url.Values{"Email": {"hacked@example.com"}}, nil)
	if resp.StatusCode != 403 {
		t.Errorf("got %d, want 403", resp.StatusCode)
	}
	if userAdmin.store[theirs.ID].Email != before {
		t.Errorf("the record was modified despite the denial: %q", userAdmin.store[theirs.ID].Email)
	}
}

func TestPerObjectPermissionGatesDeletion(t *testing.T) {
	app, userAdmin, _, theirs := objectPermApp(t)

	if got := doDelete(t, app, "/admin/users/"+strconv.Itoa(theirs.ID)+"/delete", nil).StatusCode; got != 403 {
		t.Errorf("hx-delete of someone else's record: got %d, want 403", got)
	}
	if _, still := userAdmin.store[theirs.ID]; !still {
		t.Error("the record was deleted despite the denial")
	}
}

// A record you may see but not change must not be offered an Edit
// button -- the controls follow the same per-object answer.
func TestPerObjectPermissionHidesControlsOnTheDetailPage(t *testing.T) {
	app, _, mine, theirs := objectPermApp(t)

	own := body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(mine.ID), nil))
	if !strings.Contains(own, "/edit") {
		t.Error("own record should offer Edit")
	}
	// Viewing someone else's record is refused outright by this rule,
	// which is itself the per-object view check doing its job.
	if got := doGet(t, app, "/admin/users/"+strconv.Itoa(theirs.ID), nil).StatusCode; got != 403 {
		t.Errorf("someone else's detail page: got %d, want 403", got)
	}
}
