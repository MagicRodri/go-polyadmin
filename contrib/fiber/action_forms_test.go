package fiber

import (
	"context"
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

var actionOrgRelation = core.Relation{Name: "Org", Target: "organizations", DisplayField: "Name"}

func renameDomain(ctx context.Context, ma core.ModelAdmin, objects []any, data map[string]any, p *core.Principal) (core.ActionResult, error) {
	domain, _ := data["Domain"].(string)
	if strings.HasPrefix(domain, ".") {
		return core.ActionResult{}, &core.ActionFormError{Errors: map[string][]string{"Domain": {"No leading dot."}}}
	}
	for _, obj := range objects {
		u := obj.(*testUser)
		u.Email = strings.Split(u.Email, "@")[0] + "@" + domain
	}
	return core.ActionResult{Message: "Renamed"}, nil
}

type formApp struct {
	app      *fiber.App
	users    *testUserAdmin
	orgs     *testOrgAdmin
	assigned any
}

func makeFormApp(t *testing.T) *formApp {
	t.Helper()
	fa := &formApp{users: newTestUserAdmin(), orgs: newTestOrgAdmin()}
	fa.users.DeclaredActions = []core.Action{
		core.NewAction("rename_domain", nil,
			core.WithActionLabel("Rename domain"),
			core.WithActionSubmitLabel("Rename"),
			core.WithActionForm(core.NewField("Domain", core.FieldTypeString, core.WithRequired())),
			core.WithActionFormHandler(renameDomain)),
		core.NewAction("assign_org", nil,
			core.WithActionForm(core.NewField("Org", core.FieldTypeForeignKey, core.WithRelation(actionOrgRelation), core.WithRequired())),
			core.WithActionFormHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, data map[string]any, p *core.Principal) (core.ActionResult, error) {
				fa.assigned = data["Org"]
				return core.ActionResult{}, nil
			})),
		core.NewAction("assign_org_note", nil,
			core.WithActionForm(
				core.NewField("Org", core.FieldTypeForeignKey, core.WithRelation(actionOrgRelation), core.WithRequired()),
				core.NewField("Note", core.FieldTypeString, core.WithRequired())),
			core.WithActionFormHandler(func(ctx context.Context, ma core.ModelAdmin, objects []any, data map[string]any, p *core.Principal) (core.ActionResult, error) {
				return core.ActionResult{}, nil
			})),
	}
	fa.app = newTestApp(t, core.New(core.WithModelAdmins(fa.users, fa.orgs)))
	return fa
}

func readBody(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestFormActionFirstPostRendersTheForm(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/rename_domain", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	body := readBody(t, resp.Body)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(body, `name="Domain"`) || !strings.Contains(body, `name="_confirmed" value="1"`) || !strings.Contains(body, ">Rename<") {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if fa.users.store[a.ID].Email != "a@x.com" {
		t.Fatal("handler ran on first post")
	}
}

func TestFormActionConfirmedPostRunsWithData(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/rename_domain",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Domain": {"y.org"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || fa.users.store[a.ID].Email != "a@y.org" {
		t.Fatalf("status %d email %s", resp.StatusCode, fa.users.store[a.ID].Email)
	}
}

func TestFormActionValidationRedisplays(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/rename_domain",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Domain": {""}}, nil)
	if resp.StatusCode != fiber.StatusUnprocessableEntity || !strings.Contains(readBody(t, resp.Body), "is required") {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestFormActionHandlerErrorRedisplays(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/rename_domain",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Domain": {".bad"}}, nil)
	body := readBody(t, resp.Body)
	if resp.StatusCode != fiber.StatusUnprocessableEntity || !strings.Contains(body, "No leading dot.") || !strings.Contains(body, `value=".bad"`) {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestFormActionRelationUsesTargetLookup(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/assign_org", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if !strings.Contains(readBody(t, resp.Body), "/admin/organizations/lookup") {
		t.Fatal("no lookup url")
	}
}

func TestFormActionRelationValueReachesHandler(t *testing.T) {
	fa := makeFormApp(t)
	fa.orgs.store[7] = &testOrg{ID: 7, Name: "Acme"}
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/assign_org",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Org": {"7"}}, nil)
	if resp.StatusCode != fiber.StatusSeeOther || stringOrEmpty(fa.assigned) != "7" {
		t.Fatalf("status %d assigned %v", resp.StatusCode, fa.assigned)
	}
}

func TestFormActionRelationSelectionSurvivesRedisplay(t *testing.T) {
	fa := makeFormApp(t)
	fa.orgs.store[7] = &testOrg{ID: 7, Name: "Acme Widgets"}
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/assign_org_note",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Org": {"7"}, "Note": {""}}, nil)
	body := readBody(t, resp.Body)
	if resp.StatusCode != fiber.StatusUnprocessableEntity || !strings.Contains(body, "Acme Widgets") || !strings.Contains(body, `value="7"`) {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
}

func TestFormActionSelectAllChangedDoesNotRun(t *testing.T) {
	fa := makeFormApp(t)
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/rename_domain",
		url.Values{"_select_all": {"1"}, "_confirmed": {"1"}, "_fingerprint": {"stale"}, "Domain": {"y.org"}}, nil)
	if resp.StatusCode != fiber.StatusOK || !strings.Contains(readBody(t, resp.Body), "The selection changed") {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if fa.users.store[a.ID].Email != "a@x.com" {
		t.Fatal("ran on a changed selection")
	}
}

type denyOrganizations struct{}

func (denyOrganizations) Can(p *core.Principal, permission string, resource any) bool {
	return !strings.HasPrefix(permission, "organizations.")
}

func makeDeniedFormApp(t *testing.T, searchable bool) *formApp {
	t.Helper()
	fa := makeFormApp(t)
	if !searchable {
		fa.orgs.SearchFieldNames = nil
	}
	fa.app = newTestApp(t, core.New(core.WithModelAdmins(fa.users, fa.orgs), core.WithAuthorizer(denyOrganizations{})))
	return fa
}

func TestFormActionWithholdsOptionsOfAnUnviewableTarget(t *testing.T) {
	fa := makeDeniedFormApp(t, false)
	fa.orgs.store[7] = &testOrg{ID: 7, Name: "Secret Holdings"}
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/assign_org", url.Values{"pks": {strconv.Itoa(a.ID)}}, nil)
	if body := readBody(t, resp.Body); resp.StatusCode != fiber.StatusOK || strings.Contains(body, "Secret Holdings") {
		t.Fatalf("status %d leaked option", resp.StatusCode)
	}
}

func TestFormActionWithholdsTheLabelOfAnUnviewableTarget(t *testing.T) {
	fa := makeDeniedFormApp(t, true)
	fa.orgs.store[7] = &testOrg{ID: 7, Name: "Secret Holdings"}
	a := fa.users.createUser("a@x.com", true)
	resp := doPostForm(t, fa.app, "/admin/users/actions/assign_org_note",
		url.Values{"pks": {strconv.Itoa(a.ID)}, "_confirmed": {"1"}, "Org": {"7"}, "Note": {""}}, nil)
	if body := readBody(t, resp.Body); resp.StatusCode != fiber.StatusUnprocessableEntity || strings.Contains(body, "Secret Holdings") {
		t.Fatalf("status %d leaked label", resp.StatusCode)
	}
}
