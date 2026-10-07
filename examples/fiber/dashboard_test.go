package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/contrib/fiber"

	"github.com/gofiber/fiber/v2"
)

func newDashboardTestApp(t *testing.T) *fiber.App {
	t.Helper()
	users := NewUserRepository()
	organizations := NewOrganizationRepository()
	roles := NewRoleRepository()
	seed(users, organizations, roles)
	admin := core.New(
		core.WithModelAdmins(NewUserAdmin(users, organizations, roles), NewOrganizationAdmin(organizations, users), NewRoleAdmin(roles, users)),
		core.WithDashboard(newDashboard(users, organizations, roles)),
	)
	app := fiber.New()
	if err := fiberadapter.Mount(app.Group("/admin"), admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app
}

func getDashboard(t *testing.T, app *fiber.App, path string) (string, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body), resp.Header.Get("Content-Disposition")
}

func TestDashboardWidgetsLoadThroughTheirFragments(t *testing.T) {
	app := newDashboardTestApp(t)
	page, _ := getDashboard(t, app, "/admin/")
	if !strings.Contains(page, `id="dashboard-filters"`) || !strings.Contains(page, `data-dashboard-export="users-csv"`) {
		t.Fatal("filters or export missing")
	}
	if tiles, _ := getDashboard(t, app, "/admin/_widgets/overview"); strings.Count(tiles, "data-tile") != 4 {
		t.Fatal(tiles)
	}
	if rows, _ := getDashboard(t, app, "/admin/_widgets/users?search=user00"); !strings.Contains(rows, "data-row") {
		t.Fatal(rows)
	}
	if billing, _ := getDashboard(t, app, "/admin/_widgets/billing"); !strings.Contains(billing, `data-widget-state="unavailable"`) {
		t.Fatal(billing)
	}
}

func TestDashboardExportDownloadsTheUsersCSV(t *testing.T) {
	body, disposition := getDashboard(t, newDashboardTestApp(t), "/admin/_exports/users-csv")
	if !strings.HasPrefix(disposition, `attachment; filename="users.csv"`) || !strings.Contains(body, "email") {
		t.Fatalf("%q %q", disposition, body)
	}
}
