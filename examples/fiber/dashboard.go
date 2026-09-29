package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
)

func usersIn(users *UserRepository, dc core.DashboardContext) []*User {
	selected := dc.String("organization")
	out := []*User{}
	for _, u := range users.List() {
		if selected == "" || (u.Organization != nil && strconv.Itoa(u.Organization.ID) == selected) {
			out = append(out, u)
		}
	}
	return out
}

func share(count, of int) map[string]any {
	percentage := 0.0
	if of > 0 {
		percentage = float64(count) / float64(of) * 100
	}
	return map[string]any{"count": count, "percentage": percentage}
}

func newDashboard(users *UserRepository, organizations *OrganizationRepository, roles *RoleRepository) *core.Dashboard {
	overview := core.NewMetricGroup("At a glance", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) {
		scoped := usersIn(users, wc.DashboardContext)
		active := 0
		for _, u := range scoped {
			if u.IsActive {
				active++
			}
		}
		return []core.Tile{
			{Label: "Users", Value: len(scoped), Icon: "users", Hint: fmt.Sprintf("active: %d", active)},
			{Label: "Organizations", Value: len(organizations.List()), Icon: "file-text"},
			{Label: "Roles", Value: len(roles.List()), Icon: "activity"},
			{Label: "Inactive users", Value: len(scoped) - active, Icon: "user"},
		}, nil
	}, core.WithKey("overview"), core.WithPlacement("top"), core.WithDependsOn("organization"))

	orgTable := core.NewDataTable("Organizations", []core.Column{
		{Key: "name", Label: "Organization", Strong: true},
		{Key: "users", Label: "Users", Align: "end", Format: "number"},
		{Key: "active", Label: "Active", Align: "end", Format: "share"},
	}, func(ctx context.Context, wc core.WidgetContext) (core.Rows, error) {
		items := []map[string]any{}
		totalUsers, totalActive := 0, 0
		for _, org := range organizations.List() {
			members, active := 0, 0
			for _, u := range users.List() {
				if u.Organization == org {
					members++
					if u.IsActive {
						active++
					}
				}
			}
			totalUsers += members
			totalActive += active
			items = append(items, map[string]any{"name": org.Name, "users": members, "active": share(active, members)})
		}
		return core.Rows{Items: items, Total: core.Total(len(items)),
			Totals: map[string]any{"name": "All", "users": totalUsers, "active": share(totalActive, totalUsers)}}, nil
	}, core.WithKey("organizations"), core.WithSize("lg"), core.WithDependsOn())
	orgTable.PageSizeValue = 0

	userTable := core.NewDataTable("Users", []core.Column{
		{Key: "email", Label: "Email", Strong: true},
		{Key: "organization", Label: "Organization", Empty: "none"},
		{Key: "active", Label: "Active"},
	}, func(ctx context.Context, wc core.WidgetContext) (core.Rows, error) {
		scoped := []*User{}
		for _, u := range usersIn(users, wc.DashboardContext) {
			if wc.Search == "" || strings.Contains(u.Email, wc.Search) {
				scoped = append(scoped, u)
			}
		}
		end := min(wc.Offset+wc.Limit, len(scoped))
		items := []map[string]any{}
		for _, u := range scoped[min(wc.Offset, len(scoped)):end] {
			var org any
			if u.Organization != nil {
				org = u.Organization.Name
			}
			active := "no"
			if u.IsActive {
				active = "yes"
			}
			items = append(items, map[string]any{"email": u.Email, "organization": org, "active": active})
		}
		return core.Rows{Items: items, Total: core.Total(len(scoped))}, nil
	}, core.WithKey("users"), core.WithSize("full"), core.WithDependsOn("organization"),
		core.WithDescriptionFunc(func(dc core.DashboardContext) string {
			r := dc.DateRange("period")
			return r.Start.Format("2006-01-02") + " – " + r.End.Format("2006-01-02")
		}))
	userTable.PageSizeValue = 25
	userTable.Searchable = true
	userTable.SearchPlaceholder = "Search by email"
	userTable.TotalLabel = "{total} users"

	billing := core.NewMetricGroup("Billing", func(ctx context.Context, wc core.WidgetContext) ([]core.Tile, error) {
		return nil, &core.WidgetUnavailable{Message: "The billing service is not configured in this demo."}
	}, core.WithKey("billing"), core.WithDependsOn())

	return &core.Dashboard{
		Title: "Overview",
		Filters: []core.DashboardFilter{
			core.NewDateRangeFilter("period", 30, core.WithFilterLabel("Period")),
			core.NewSelectFilter("organization", core.WithFilterLabel("Organization"), core.WithEmptyLabel("All organizations"), core.WithSearchableSelect(),
				core.WithFilterChoicesFunc(func(ctx context.Context) ([]core.Choice, error) {
					choices := []core.Choice{}
					for _, o := range organizations.List() {
						choices = append(choices, core.Choice{Value: strconv.Itoa(o.ID), Label: o.Name})
					}
					return choices, nil
				})),
		},
		Exports: []core.DashboardExport{{Name: "users-csv", Label: "Export users",
			Handler: func(ctx context.Context, dc core.DashboardContext) (*core.Download, error) {
				lines := []string{"email"}
				for _, u := range usersIn(users, dc) {
					lines = append(lines, u.Email)
				}
				return &core.Download{Filename: "users.csv", ContentType: "text/csv", Content: []byte(strings.Join(lines, "\n"))}, nil
			}}},
		Widgets: []core.Widget{
			overview,
			orgTable,
			userTable,
			billing,
			core.NewMetric("Users", func() any { return len(users.List()) }, core.WithKey("user-count")),
			core.NewMetric("Organizations", func() any { return len(organizations.List()) }, core.WithKey("organization-count")),
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
}
