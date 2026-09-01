// Reference Fiber application exercising the PolyAdmin package.
//
// Run with:
//
//	go run .
//
// then open http://127.0.0.1:3000/admin
package main

import (
	"fmt"
	"log"
	"sort"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/fiber"

	"github.com/gofiber/fiber/v2"
)

func main() {
	users := NewUserRepository()
	organizations := NewOrganizationRepository()
	roles := NewRoleRepository()
	seed(users, organizations, roles)

	dashboard := &core.Dashboard{
		Title: "Overview",
		Widgets: []core.Widget{
			core.NewMetric("Users", func() any { return len(users.List()) }),
			core.NewMetric("Organizations", func() any { return len(organizations.List()) }),
			// Stat pairs the number with its trend. A real app would
			// get the delta by comparing against a previous-period
			// query; these demo repositories keep no history, so it's a
			// fixed stand-in here.
			core.NewStat("Active users", func() (any, float64) {
				active := 0
				for _, u := range users.List() {
					if u.IsActive {
						active++
					}
				}
				return active, 12.5
			}),
			// Two breakdowns of the same population sharing one card.
			// Each panel is an ordinary widget; all of them render up
			// front, so switching tabs costs no round trip.
			core.NewTabs("User breakdown", []core.TabPanel{
				{Label: "By status", Widget: core.NewDonut("Users by status", func() []core.ChartPoint {
					active, inactive := 0, 0
					for _, u := range users.List() {
						if u.IsActive {
							active++
						} else {
							inactive++
						}
					}
					return []core.ChartPoint{
						{Label: "Active", Value: float64(active)},
						{Label: "Inactive", Value: float64(inactive)},
					}
				})},
				{Label: "By organization", Widget: core.NewDonut("Users by organization", func() []core.ChartPoint {
					points := make([]core.ChartPoint, 0, len(organizations.List())+1)
					unassigned := 0
					for _, org := range organizations.List() {
						count := 0
						for _, u := range users.List() {
							if u.Organization == org {
								count++
							}
						}
						points = append(points, core.ChartPoint{Label: org.Name, Value: float64(count)})
					}
					for _, u := range users.List() {
						if u.Organization == nil {
							unassigned++
						}
					}
					points = append(points, core.ChartPoint{Label: "Unassigned", Value: float64(unassigned)})
					return points
				})},
			}),
			core.NewTable("Recent users", []string{"Email", "Organization"}, func() []map[string]any {
				rows := make([]map[string]any, 0)
				for _, u := range users.List() {
					orgName := "—"
					if u.Organization != nil {
						orgName = u.Organization.Name
					}
					rows = append(rows, map[string]any{"Email": u.Email, "Organization": orgName})
				}
				return rows
			}),
			// Timeline is Activity's richer sibling: each entry carries
			// its own timestamp and body. A real app would order by a
			// created_at column and format the time however it likes --
			// the widget only ever displays the string it's given.
			core.NewTimeline("Latest activity", func() []core.TimelineEntry {
				newest := users.List()
				sort.Slice(newest, func(i, j int) bool { return newest[i].ID > newest[j].ID })
				entries := make([]core.TimelineEntry, 0, 3)
				for _, u := range newest {
					if len(entries) == 3 {
						break
					}
					entries = append(entries, core.TimelineEntry{
						Time:        fmt.Sprintf("user #%d", u.ID),
						Title:       "Account created",
						Description: u.Email,
					})
				}
				return entries
			}),
		},
	}

	// Cookie sessions over an in-memory user table (session.go). One
	// object serves as both halves: WithLoginBackend is what mounts the
	// admin's login page and makes an unauthenticated request redirect
	// to it, and WithAuthenticator is what reads the session back on
	// every subsequent request.
	//
	// This replaced an AllowAllAuthenticator hardcoded to a superuser,
	// which meant nothing below -- SuperuserAuthorizer, per-object
	// permissions, the audit log's principal -- was ever exercised
	// against an identity anyone actually proved.
	sessions := NewCookieSessionBackend()
	admin := core.New(
		core.WithModelAdmins(NewUserAdmin(users, organizations, roles), NewOrganizationAdmin(organizations), NewRoleAdmin(roles)),
		core.WithDashboard(dashboard),
		core.WithAuthenticator(sessions),
		core.WithLoginBackend(sessions),
		// Not core.SuperuserAuthorizer: that would deny the viewer
		// account every permission, dashboard included. See session.go.
		core.WithAuthorizer(ReadOnlyForNonSuperusers{}),
	)
	registerPages(admin, users)

	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"admin": "/admin"})
	})

	group := app.Group("/admin")
	if err := fiberadapter.Mount(group, admin, "/admin", fiberadapter.WithTemplateDirs("templates")); err != nil {
		log.Fatal(err)
	}

	log.Fatal(app.Listen(":3000"))
}
