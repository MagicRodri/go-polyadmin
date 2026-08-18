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

// Distinct names from relations_test.go's/router_test.go's own test
// doubles (same package) -- this file's own self-contained
// parent/child pair for inline-specific scenarios.

type inlineTestOrg struct {
	ID   int
	Name string
}

type inlineTestOrgAdmin struct {
	core.BaseModelAdmin
	store  map[int]*inlineTestOrg
	nextID int
}

func newInlineTestOrgAdmin(layout string) *inlineTestOrgAdmin {
	var inline core.Inline
	if layout == core.InlineLayoutStacked {
		inline = core.NewStackedInline("users", "Organization")
	} else {
		inline = core.NewTabularInline("users", "Organization")
	}
	return &inlineTestOrgAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:       "Organization",
			SlugOverride:    "organizations",
			DisplayFields:   []string{"ID", "Name"},
			FormFieldNames:  []string{"Name"},
			DeclaredFields:  []core.Field{core.NewField("Name", core.FieldTypeString, core.WithRequired())},
			DeclaredInlines: []core.Inline{inline},
		},
		store:  make(map[int]*inlineTestOrg),
		nextID: 1,
	}
}

func (a *inlineTestOrgAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, o := range a.store {
		out = append(out, o)
	}
	return out, nil
}

func (a *inlineTestOrgAdmin) GetObject(ctx context.Context, pk any) (any, error) {
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

func (a *inlineTestOrgAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	o := &inlineTestOrg{ID: a.nextID, Name: data["Name"].(string)}
	a.store[o.ID] = o
	a.nextID++
	return o, nil
}

type inlineTestUser struct {
	ID           int
	Email        string
	IsActive     bool
	Organization *inlineTestOrg
}

var inlineTestOrgRelation = core.Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}

type inlineTestUserAdmin struct {
	core.BaseModelAdmin
	store    map[int]*inlineTestUser
	nextID   int
	orgAdmin *inlineTestOrgAdmin
}

func newInlineTestUserAdmin(orgAdmin *inlineTestOrgAdmin) *inlineTestUserAdmin {
	return &inlineTestUserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "IsActive", "Organization"},
			DetailFieldNames: []string{"ID", "Email", "IsActive", "Organization"},
			FormFieldNames:   []string{"Email", "IsActive", "Organization"},
			DeclaredFields: []core.Field{
				core.NewField("Email", core.FieldTypeEmail, core.WithRequired()),
				core.NewField("IsActive", core.FieldTypeBoolean),
				core.NewField("Organization", core.FieldTypeForeignKey, core.WithRelation(inlineTestOrgRelation)),
			},
		},
		store:    make(map[int]*inlineTestUser),
		nextID:   1,
		orgAdmin: orgAdmin,
	}
}

func (a *inlineTestUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, u := range a.store {
		out = append(out, u)
	}
	return out, nil
}

func (a *inlineTestUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
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

func (a *inlineTestUserAdmin) resolveOrg(data map[string]any) *inlineTestOrg {
	raw, _ := data["Organization"].(string)
	if raw == "" {
		return nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return a.orgAdmin.store[id]
}

func (a *inlineTestUserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	email, _ := data["Email"].(string)
	active, _ := data["IsActive"].(bool)
	u := &inlineTestUser{ID: a.nextID, Email: email, IsActive: active, Organization: a.resolveOrg(data)}
	a.store[u.ID] = u
	a.nextID++
	return u, nil
}

func (a *inlineTestUserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	u := obj.(*inlineTestUser)
	if email, ok := data["Email"].(string); ok {
		u.Email = email
	}
	if active, ok := data["IsActive"].(bool); ok {
		u.IsActive = active
	}
	u.Organization = a.resolveOrg(data)
	return u, nil
}

func (a *inlineTestUserAdmin) Delete(ctx context.Context, obj any) error {
	delete(a.store, obj.(*inlineTestUser).ID)
	return nil
}

func newInlineTestApp(t *testing.T, layout string, opts ...core.Option) (*fiber.App, *inlineTestOrgAdmin, *inlineTestUserAdmin) {
	t.Helper()
	orgAdmin := newInlineTestOrgAdmin(layout)
	userAdmin := newInlineTestUserAdmin(orgAdmin)
	admin := core.New(append([]core.Option{core.WithModelAdmins(orgAdmin, userAdmin)}, opts...)...)
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app, orgAdmin, userAdmin
}

func seedOrgWithUsers(orgAdmin *inlineTestOrgAdmin, userAdmin *inlineTestUserAdmin, emails ...string) (*inlineTestOrg, []*inlineTestUser) {
	org, _ := orgAdmin.Create(context.Background(), map[string]any{"Name": "Acme"})
	o := org.(*inlineTestOrg)
	var users []*inlineTestUser
	for _, email := range emails {
		obj, _ := userAdmin.Create(context.Background(), map[string]any{
			"Email": email, "IsActive": true, "Organization": strconv.Itoa(o.ID),
		})
		users = append(users, obj.(*inlineTestUser))
	}
	return o, users
}

func TestInlineSectionRendersOnEditPage(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	otherOrg, _ := seedOrgWithUsers(orgAdmin, userAdmin, "outsider@example.com")
	_ = otherOrg

	resp := doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)
	text := body(t, resp)
	if !strings.Contains(text, `id="inline-users"`) {
		t.Fatalf("expected inline section, got %s", text)
	}
	if !strings.Contains(text, "a@example.com") {
		t.Fatalf("expected seeded user, got %s", text)
	}
	if strings.Contains(text, "outsider@example.com") {
		t.Fatalf("did not expect another org's user, got %s", text)
	}
}

func TestInlineSectionPlaceholderOnCreatePage(t *testing.T) {
	app, _, _ := newInlineTestApp(t, core.InlineLayoutTabular)

	resp := doGet(t, app, "/admin/organizations/create", nil)
	text := body(t, resp)
	if !strings.Contains(text, "Save to add") {
		t.Fatalf("expected placeholder text, got %s", text)
	}
}

func TestInlineSectionReadonlyOnDetailPage(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	resp := doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil)
	text := body(t, resp)
	section := strings.Split(text, `id="inline-users"`)[1]
	if !strings.Contains(section, "a@example.com") {
		t.Fatalf("expected seeded user, got %s", section)
	}
	if strings.Contains(section, "<input") {
		t.Fatalf("expected no inputs on a readonly section, got %s", section)
	}
}

func TestInlineCreateAddsRowAndReturnsSectionFragment(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin)

	form := url.Values{"Email": {"new@example.com"}, "IsActive": {"true"}}
	resp := doPostForm(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/inlines/users", form, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	text := body(t, resp)
	if strings.Contains(strings.ToLower(text), "<html") {
		t.Fatalf("expected a bare fragment, got %s", text)
	}
	if !strings.Contains(text, `id="inline-users"`) || !strings.Contains(text, "new@example.com") {
		t.Fatalf("got %s", text)
	}
	if len(userAdmin.store) != 1 {
		t.Fatalf("expected 1 user, got %d", len(userAdmin.store))
	}
	for _, u := range userAdmin.store {
		if u.Organization == nil || u.Organization.ID != org.ID {
			t.Fatalf("expected user linked to org, got %+v", u)
		}
	}
}

func TestInlineCreateValidationErrorReturns422(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin)

	resp := doPostForm(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/inlines/users", url.Values{"Email": {""}}, nil)
	if resp.StatusCode != 422 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected no user created, got %d", len(userAdmin.store))
	}
}

func TestInlineUpdateEditsExistingRow(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, users := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/inlines/users/" + strconv.Itoa(users[0].ID)
	resp := doPostForm(t, app, path, url.Values{"Email": {"changed@example.com"}, "IsActive": {"true"}}, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if userAdmin.store[users[0].ID].Email != "changed@example.com" {
		t.Fatalf("got %q", userAdmin.store[users[0].ID].Email)
	}
}

func TestInlineDeleteRemovesRow(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, users := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/inlines/users/" + strconv.Itoa(users[0].ID)
	resp := doDelete(t, app, path, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected user removed, got %d", len(userAdmin.store))
	}
}

type denyChildPermissionAuthorizer struct{ denied string }

func (d denyChildPermissionAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != d.denied
}

func TestInlineCreateDeniedWithoutChildCreatePermission(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular,
		core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)),
		core.WithAuthorizer(denyChildPermissionAuthorizer{denied: "users.create"}),
	)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin)

	resp := doPostForm(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/inlines/users", url.Values{"Email": {"x@example.com"}}, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected no user created, got %d", len(userAdmin.store))
	}
}

func TestInlineUpdateDeniedWithoutChildUpdatePermission(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular,
		core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)),
		core.WithAuthorizer(denyChildPermissionAuthorizer{denied: "users.update"}),
	)
	org, users := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/inlines/users/" + strconv.Itoa(users[0].ID)
	resp := doPostForm(t, app, path, url.Values{"Email": {"changed@example.com"}}, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if userAdmin.store[users[0].ID].Email != "a@example.com" {
		t.Fatalf("got %q", userAdmin.store[users[0].ID].Email)
	}
}

func TestInlineDeleteDeniedWithoutChildDeletePermission(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular,
		core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)),
		core.WithAuthorizer(denyChildPermissionAuthorizer{denied: "users.delete"}),
	)
	org, users := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/inlines/users/" + strconv.Itoa(users[0].ID)
	resp := doDelete(t, app, path, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if len(userAdmin.store) != 1 {
		t.Fatalf("expected user to remain, got %d", len(userAdmin.store))
	}
}

func TestInlineMutationDeniedWithoutParentUpdatePermission(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular,
		core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)),
		core.WithAuthorizer(denyChildPermissionAuthorizer{denied: "organizations.update"}),
	)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin)

	resp := doPostForm(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/inlines/users", url.Values{"Email": {"x@example.com"}}, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestInlineSectionHiddenWithoutChildViewPermission(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular,
		core.WithAuthenticator(core.NewAllowAllAuthenticator(nil)),
		core.WithAuthorizer(denyChildPermissionAuthorizer{denied: "users.view"}),
	)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	resp := doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)
	text := body(t, resp)
	if strings.Contains(text, `id="inline-users"`) {
		t.Fatalf("expected inline section to be hidden entirely, got %s", text)
	}
}

func TestStackedInlineRendersFormPerRow(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutStacked)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	resp := doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)
	text := body(t, resp)
	section := strings.Split(text, `id="inline-users"`)[1]
	if strings.Contains(section, "<table") {
		t.Fatalf("did not expect a table in the stacked layout, got %s", section)
	}
	if !strings.Contains(section, "<form") {
		t.Fatalf("expected a per-row form in the stacked layout, got %s", section)
	}
}

func TestTabularInlineRendersTable(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	resp := doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)
	text := body(t, resp)
	section := strings.Split(text, `id="inline-users"`)[1]
	if !strings.Contains(section, "<table") {
		t.Fatalf("expected a table in the tabular layout, got %s", section)
	}
}

func TestMountReturnsErrorOnDuplicateInlineChildSlug(t *testing.T) {
	orgAdmin := &inlineTestOrgAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:    "Organization",
			SlugOverride: "organizations",
			DeclaredInlines: []core.Inline{
				core.NewStackedInline("users", "Organization"),
				core.NewTabularInline("users", "Organization"),
			},
		},
		store: make(map[int]*inlineTestOrg),
	}
	userAdmin := newInlineTestUserAdmin(orgAdmin)
	admin := core.New(core.WithModelAdmins(orgAdmin, userAdmin))
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin"); err == nil {
		t.Fatalf("expected an error for duplicate inline child slug")
	}
}

func TestMountReturnsErrorWhenInlineFKFieldDoesNotTargetParent(t *testing.T) {
	orgAdmin := &inlineTestOrgAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:       "Organization",
			SlugOverride:    "organizations",
			DeclaredInlines: []core.Inline{core.NewStackedInline("users", "Email")}, // "Email" isn't a relation field at all
		},
		store: make(map[int]*inlineTestOrg),
	}
	userAdmin := newInlineTestUserAdmin(orgAdmin)
	admin := core.New(core.WithModelAdmins(orgAdmin, userAdmin))
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin"); err == nil {
		t.Fatalf("expected an error for a bad inline fk_field")
	}
}
