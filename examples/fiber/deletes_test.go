package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/fiber"

	"github.com/gofiber/fiber/v2"
)

// newSeededApp mounts the three admins over freshly seeded repositories,
// with no authenticator: every request is permitted (as in
// newOrganizationTestApp).
func newSeededApp(t *testing.T) (*fiber.App, *OrganizationRepository, *UserRepository, *RoleRepository) {
	t.Helper()
	orgs, users, roles := NewOrganizationRepository(), NewUserRepository(), NewRoleRepository()
	seed(users, orgs, roles)
	admin := core.New(core.WithModelAdmins(
		NewUserAdmin(users, orgs, roles), NewOrganizationAdmin(orgs, users), NewRoleAdmin(roles, users),
	))
	app := fiber.New()
	if err := fiberadapter.Mount(app.Group("/admin"), admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app, orgs, users, roles
}

func getPage(t *testing.T, app *fiber.App, path string) string {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestOrganizationDeletePageListsItsUsers(t *testing.T) {
	app, _, _, _ := newSeededApp(t)
	page := getPage(t, app, "/admin/organizations/4/delete") // Initech: samir
	for _, want := range []string{"This will also delete", "User (1)", "samir@example.com"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestDeletingAnOrganizationDeletesItsUsers(t *testing.T) {
	app, orgs, users, _ := newSeededApp(t)
	resp := postOrganizationForm(t, app, "/admin/organizations/4/delete", url.Values{})
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if orgs.Get(4) != nil {
		t.Error("organization survived")
	}
	for _, u := range users.List() {
		if u.Email == "samir@example.com" {
			t.Error("its user survived")
		}
	}
}

func TestAnAssignedRoleCannotBeDeleted(t *testing.T) {
	app, _, _, roles := newSeededApp(t)
	page := getPage(t, app, "/admin/roles/2/delete") // Billing: jane, mary
	if !strings.Contains(page, "This can&#39;t be deleted") || !strings.Contains(page, "jane@example.com") {
		t.Error("the assigned role's page does not say why it is blocked")
	}
	postOrganizationForm(t, app, "/admin/roles/2/delete", url.Values{})
	if roles.Get(2) == nil {
		t.Error("an assigned role was deleted")
	}
}

func TestAnUnassignedRoleCanBeDeleted(t *testing.T) {
	app, _, _, roles := newSeededApp(t)
	postOrganizationForm(t, app, "/admin/roles/4/delete", url.Values{}) // Auditor
	if roles.Get(4) != nil {
		t.Error("an unassigned role survived")
	}
}
