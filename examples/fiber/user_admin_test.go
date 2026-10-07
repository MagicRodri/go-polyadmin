package main

import (
	"io"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/contrib/fiber"

	"github.com/gofiber/fiber/v2"
)

func newUserTestApp(t *testing.T) (*fiber.App, *UserRepository, *OrganizationRepository, *RoleRepository) {
	t.Helper()
	users := NewUserRepository()
	organizations := NewOrganizationRepository()
	roles := NewRoleRepository()
	admin := core.New(core.WithModelAdmins(
		NewUserAdmin(users, organizations, roles),
		NewOrganizationAdmin(organizations, users),
		NewRoleAdmin(roles, users),
	))
	app := fiber.New()
	if err := fiberadapter.Mount(app.Group("/admin"), admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app, users, organizations, roles
}

func TestBulkEditChangesThePlanAndKeepsEverythingElse(t *testing.T) {
	app, users, organizations, roles := newUserTestApp(t)
	org := organizations.Create("Acme", time.Time{}, 0)
	role := roles.Create("Support")
	user := users.Create("a@example.com", true, "Free", org, []any{role})
	resp := postOrganizationForm(t, app, "/admin/users/actions/bulk_edit", url.Values{
		"pks": {strconv.Itoa(user.ID)}, "_confirmed": {"1"}, "_change_Plan": {"1"}, "Plan": {"Enterprise"},
	})
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if user.Plan != "Enterprise" || user.Email != "a@example.com" || !user.IsActive || user.Organization != org || len(user.Roles) != 1 {
		t.Fatalf("user %+v", user)
	}
}

func TestExportEmailsDownloadsACSV(t *testing.T) {
	app, users, _, _ := newUserTestApp(t)
	user := users.Create("a@example.com", true, "Free", nil, nil)
	resp := postOrganizationForm(t, app, "/admin/users/actions/export_emails", url.Values{"pks": {strconv.Itoa(user.ID)}})
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), `attachment; filename="users.csv"`) || !strings.Contains(string(body), "a@example.com") {
		t.Fatalf("headers %v body %s", resp.Header, body)
	}
}

func TestAssignOrganizationMovesTheSelectedUsers(t *testing.T) {
	app, users, organizations, _ := newUserTestApp(t)
	org := organizations.Create("Acme", time.Time{}, 0)
	user := users.Create("a@example.com", true, "Free", nil, nil)
	resp := postOrganizationForm(t, app, "/admin/users/actions/assign_organization", url.Values{
		"pks": {strconv.Itoa(user.ID)}, "_confirmed": {"1"}, "Organization": {strconv.Itoa(org.ID)},
	})
	if resp.StatusCode != fiber.StatusSeeOther || user.Organization != org {
		t.Fatalf("status %d org %v", resp.StatusCode, user.Organization)
	}
}
