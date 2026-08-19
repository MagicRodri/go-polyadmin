package fiber

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestNoDashboardConfiguredRedirectsToFirstResource(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 307 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestDashboardRendersAtAdminRoot(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.createUser("a@example.com", true)
	userAdmin.createUser("b@example.com", true)

	dashboard := &core.Dashboard{
		Title:   "Overview",
		Widgets: []core.Widget{core.NewMetric("Users", func() any { return len(userAdmin.store) })},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	text := body(t, resp)
	if !strings.Contains(text, "Overview") || !strings.Contains(text, ">2<") {
		t.Fatalf("got %s", text)
	}
}

func TestDashboardOmitsWidgetDeniedByAuthorizer(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{
			core.NewMetric("Users", func() any { return 1 }),
			core.NewMetric("Revenue", func() any { return 1 }, core.WithPermission("analytics.revenue.view")),
		},
	}
	authorizer := denyRevenueAuthorizer{}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard), core.WithAuthorizer(authorizer))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	text := body(t, resp)
	if !strings.Contains(text, "Users") || strings.Contains(text, "Revenue") {
		t.Fatalf("got %s", text)
	}
}

type denyRevenueAuthorizer struct{}

func (denyRevenueAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "analytics.revenue.view"
}

func TestDashboardViewRequiresDashboardPermission(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{Widgets: []core.Widget{core.NewMetric("Users", func() any { return 1 })}}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard), core.WithAuthorizer(denyDashboardAuthorizer{}))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != 403 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

type denyDashboardAuthorizer struct{}

func (denyDashboardAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != core.DashboardView
}

// The tabs widget is the only one whose body is assembled by the
// renderer rather than by its own template, so cover the round trip:
// each panel's child widget must actually reach the page.
func TestDashboardRendersNestedTabsPanels(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{core.NewTabs("Statistics", []core.TabPanel{
			{Label: "Top products", Widget: core.NewTable("Products", []string{"Name"}, func() []map[string]any {
				return []map[string]any{{"Name": "Widget Pro"}}
			})},
			{Label: "Top customers", Widget: core.NewActivity("Customers", func() []string { return []string{"a@example.com"} })},
		})},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	text := body(t, doGet(t, app, "/admin/", nil))
	for _, want := range []string{"Top products", "Top customers", "Widget Pro", "a@example.com", `role="tablist"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestDashboardStatRendersDeltaDirection(t *testing.T) {
	userAdmin := newTestUserAdmin()
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{core.NewStat("Sales", func() (any, float64) { return "$45,385", -12.5 })},
	}
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)

	text := body(t, doGet(t, app, "/admin/", nil))
	// A negative delta reads in the destructive token (not a literal
	// red), so it follows whichever theme is active -- see ui.go.
	if !strings.Contains(text, "$45,385") || !strings.Contains(text, "12.5%") || !strings.Contains(text, "text-destructive") {
		t.Fatalf("got %s", text)
	}
}
