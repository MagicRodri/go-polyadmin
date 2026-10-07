package fiber

import (
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

func placementApp(t *testing.T, actions []core.Action, detail []string) (*fiber.App, *testUserAdmin) {
	t.Helper()
	ua := newTestUserAdmin()
	ua.DeclaredActions = actions
	ua.DeclaredDetailActions = detail
	return newTestApp(t, core.New(core.WithModelAdmins(ua))), ua
}

func detailPage(t *testing.T, app *fiber.App, ua *testUserAdmin) string {
	t.Helper()
	u := ua.createUser("a@example.com", true)
	return body(t, doGet(t, app, "/admin/users/"+strconv.Itoa(u.ID), nil))
}

func TestDetailPageNeverOffersDeleteSelected(t *testing.T) {
	app, ua := placementApp(t, nil, nil)
	page := detailPage(t, app, ua)
	if strings.Contains(page, "actions/delete_selected") || strings.Contains(page, "Delete selected") {
		t.Error("the detail page offers delete_selected")
	}
}

func TestListPageOffersListActionsButNotDetailOnlyOnes(t *testing.T) {
	app, _ := placementApp(t, []core.Action{
		core.NewAction("ping", deactivateHandler, core.WithActionLabel("Ping all")),
		core.NewAction("sync", deactivateHandler, core.WithActionLabel("Detail sync"), core.WithActionWhere(core.ActionWhereDetail)),
	}, nil)
	page := body(t, doGet(t, app, "/admin/users", nil))
	for _, want := range []string{"Delete selected", "Ping all"} {
		if !strings.Contains(page, want) {
			t.Errorf("list page lacks %q", want)
		}
	}
	if strings.Contains(page, "Detail sync") {
		t.Error("list page offers a detail-only action")
	}
}

func TestDetailPageOffersActionsPlacedOnIt(t *testing.T) {
	app, ua := placementApp(t, []core.Action{
		core.NewAction("ping", deactivateHandler),
		core.NewAction("sync", deactivateHandler, core.WithActionWhere(core.ActionWhereDetail)),
		core.NewAction("bulk", deactivateHandler, core.WithActionWhere(core.ActionWhereList)),
	}, nil)
	page := detailPage(t, app, ua)
	for _, want := range []string{"actions/ping", "actions/sync"} {
		if !strings.Contains(page, want) {
			t.Errorf("detail page lacks %q", want)
		}
	}
	if strings.Contains(page, "actions/bulk") {
		t.Error("detail page offers a list-only action")
	}
}

func TestDetailActionsAllowlistLimitsTheDetailPage(t *testing.T) {
	app, ua := placementApp(t, []core.Action{
		core.NewAction("ping", deactivateHandler),
		core.NewAction("sync", deactivateHandler),
	}, []string{"sync"})
	page := detailPage(t, app, ua)
	if !strings.Contains(page, "actions/sync") || strings.Contains(page, "actions/ping") {
		t.Error("detail page does not follow the allowlist")
	}
}

func TestPlacementIsNotAuthorization(t *testing.T) {
	app, ua := placementApp(t, []core.Action{
		core.NewAction("sync", deactivateHandler, core.WithActionWhere(core.ActionWhereDetail)),
	}, nil)
	u := ua.createUser("a@example.com", true)
	resp := doPostForm(t, app, "/admin/users/actions/sync", url.Values{"pks": {strconv.Itoa(u.ID)}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Errorf("got %d", resp.StatusCode)
	}
}
