package fiber

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

type testOrg struct {
	ID   int
	Name string
}

type testOrgAdmin struct {
	core.BaseModelAdmin
	store map[int]*testOrg
}

func newTestOrgAdmin() *testOrgAdmin {
	return &testOrgAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "Organization",
			SlugOverride:     "organizations",
			DisplayFields:    []string{"ID", "Name"},
			FormFieldNames:   []string{"Name"},
			SearchFieldNames: []string{"Name"},
			DeclaredFields:   []core.Field{core.NewField("Name", core.FieldTypeString)},
		},
		store: make(map[int]*testOrg),
	}
}

func (a *testOrgAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, o := range a.store {
		out = append(out, o)
	}
	return out, nil
}

func (a *testOrgAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	o, ok := a.store[id]
	if !ok {
		return nil, nil
	}
	return o, nil
}

type relUser struct {
	ID           int
	Email        string
	Organization *testOrg
}

var orgRelation = core.Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}

type relUserAdmin struct {
	core.BaseModelAdmin
	store  map[int]*relUser
	nextID int
}

func newRelUserAdmin() *relUserAdmin {
	return &relUserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "Organization"},
			DetailFieldNames: []string{"ID", "Email", "Organization"},
			FormFieldNames:   []string{"Email", "Organization"},
			DeclaredFields: []core.Field{
				core.NewField("Email", core.FieldTypeEmail),
				core.NewField("Organization", core.FieldTypeForeignKey, core.WithRelation(orgRelation)),
			},
		},
		store:  make(map[int]*relUser),
		nextID: 1,
	}
}

func (a *relUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, u := range a.store {
		out = append(out, u)
	}
	return out, nil
}

func (a *relUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	u, ok := a.store[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func makeRelationApp(t *testing.T) (*fiber.App, *relUserAdmin, *testOrgAdmin) {
	t.Helper()
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)
	return app, userAdmin, orgAdmin
}

func TestListRendersRelatedLink(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, `href="/admin/organizations/1"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, ">Acme<") {
		t.Fatalf("got %s", text)
	}
}

func TestListShowsDashWhenRelationIsNone(t *testing.T) {
	app, userAdmin, _ := makeRelationApp(t)
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com"}

	resp := doGet(t, app, "/admin/users", nil)
	if !strings.Contains(body(t, resp), "&mdash;") {
		t.Fatalf("expected a dash for the empty relation")
	}
}

func TestDetailRendersRelatedLink(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1", nil)
	text := body(t, resp)
	if !strings.Contains(text, `href="/admin/organizations/1"`) || !strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
}

func TestCreateFormShowsRelationSelectWithOptions(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}

	resp := doGet(t, app, "/admin/users/create", nil)
	text := body(t, resp)
	if !strings.Contains(text, `name="Organization"`) || !strings.Contains(text, ">Acme<") {
		t.Fatalf("got %s", text)
	}
}

func TestEditFormPreselectsCurrentRelation(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1/edit", nil)
	text := body(t, resp)
	if !strings.Contains(text, `value="1" selected`) {
		t.Fatalf("got %s", text)
	}
}

func TestLookupRouteReturnsMatchingOptions(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[2] = &testOrg{ID: 2, Name: "Widgets Inc"}

	resp := doGet(t, app, "/admin/organizations/lookup?q=acme", nil)
	text := body(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if !strings.Contains(text, `data-pk="1"`) || !strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
	if strings.Contains(text, "Widgets Inc") {
		t.Fatalf("got %s", text)
	}
}

type denyOrgViewAuthorizer struct{}

func (denyOrgViewAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "organizations.view"
}

func TestLookupRouteRespectsAuthorization(t *testing.T) {
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin), core.WithAuthorizer(denyOrgViewAuthorizer{}))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/organizations/lookup?q=acme", nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

type autocompleteRelUserAdmin struct {
	relUserAdmin
}

func newAutocompleteRelUserAdmin() *autocompleteRelUserAdmin {
	a := &autocompleteRelUserAdmin{relUserAdmin: *newRelUserAdmin()}
	a.AutocompleteFieldNames = []string{"Organization"}
	return a
}

func TestAutocompleteFieldRendersCommandNotSelect(t *testing.T) {
	userAdmin := newAutocompleteRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/users/create", nil)
	text := body(t, resp)
	if !strings.Contains(text, "selectItem(") {
		t.Fatalf("expected the command widget's selectItem call, got %s", text)
	}
	if !strings.Contains(text, `id="combobox-results-Organization"`) {
		t.Fatalf("got %s", text)
	}
	if strings.Contains(text, `<select id="field-Organization"`) {
		t.Fatalf("expected no plain <select> for an autocomplete field, got %s", text)
	}
	// The target's full queryset must not be dumped into the page --
	// that's the whole point of routing this field through /lookup.
	if strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
}

func TestAutocompleteFieldPrefillsSelectionOnEdit(t *testing.T) {
	userAdmin := newAutocompleteRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/users/1/edit", nil)
	text := body(t, resp)
	if !strings.Contains(text, `value="Acme"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, `value="1"`) {
		t.Fatalf("got %s", text)
	}
}

func TestRelatedLinkHiddenWhenTargetNotViewable(t *testing.T) {
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	orgAdmin.BaseModelAdmin.DisableView = true
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1", nil)
	text := body(t, resp)
	if strings.Contains(text, `href="/admin/organizations/1"`) {
		t.Fatalf("expected no link, got %s", text)
	}
	if !strings.Contains(text, "Acme") {
		t.Fatalf("expected plain-text label still shown, got %s", text)
	}
}
