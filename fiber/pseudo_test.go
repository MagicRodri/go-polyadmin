package fiber

import (
	"html"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// sweepAreas turns on one group of pages at a time, so each conversion
// commit leaves the suite green. Every area is on by the end of Task 7
// except "errors", which Task 9 turns on.
var sweepAreas = map[string]bool{
	"layout": true,
	"list":   false,
	"forms":  false,
	"detail": false,
	"errors": false,
}

var (
	sweepStrip   = regexp.MustCompile(`(?is)<script\b.*?</script>|<style\b.*?</style>|<svg\b.*?</svg>|<!--.*?-->`)
	sweepVisible = regexp.MustCompile(`\s(?:placeholder|aria-label|title|alt|hx-confirm)="([^"]*)"`)
	sweepAttrVal = regexp.MustCompile(`=(?:"[^"]*"|'[^']*')`)
	sweepTag     = regexp.MustCompile(`<[^>]*>`)
	sweepPseudo  = regexp.MustCompile(`\[[^\[\]]*\]`)
	sweepLetter  = regexp.MustCompile(`\p{L}`)
)

// untranslated returns the visible strings on a page rendered in the
// pseudo-locale that are not in pseudo form. Visible means text nodes
// plus the attributes a user reads or hears. allow lists strings that are
// deliberately untranslated: data values and locale names.
func untranslated(page string, allow ...string) []string {
	page = sweepStrip.ReplaceAllString(page, " ")
	var runs []string
	for _, m := range sweepVisible.FindAllStringSubmatch(page, -1) {
		runs = append(runs, m[1])
	}
	// Blank every attribute value before splitting on tags: Alpine
	// expressions contain ">" and would otherwise split a tag in two.
	page = sweepAttrVal.ReplaceAllString(page, `=""`)
	runs = append(runs, sweepTag.Split(page, -1)...)

	var out []string
	for _, run := range runs {
		text := strings.TrimSpace(html.UnescapeString(run))
		rest := text
		// Nested brackets come from a translated argument inside a
		// translated sentence; strip innermost first until stable.
		for {
			next := sweepPseudo.ReplaceAllString(rest, "")
			if next == rest {
				break
			}
			rest = next
		}
		for _, a := range allow {
			rest = strings.ReplaceAll(rest, a, "")
		}
		if sweepLetter.MatchString(rest) {
			out = append(out, text)
		}
	}
	return out
}

func TestUntranslatedHelper(t *testing.T) {
	page := `<html><head><title>[Üšérš] · [Àdmîñ]</title><script>var x = "Hello";</script></head>
<body x-data="{ open: a > b }"><p>[Šàvé]</p><p>Save</p><input placeholder="Search"><td>a@example.com</td>
<span>[Délété [Üšér]]</span><button aria-label="[Çlöšé]"></button></body></html>`
	got := untranslated(page, "a@example.com")
	want := []string{"Search", "Save"}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// localeNames are the switcher's entries, never translated by design.
var localeNames = []string{"English", "Français", "Русский", "Pseudo (en-XA)"}

type sweepPage struct {
	area, name, path string
	headers          map[string]string
	app              func(t *testing.T) (*fiber.App, []string)
}

func pseudoCookie(extra map[string]string) map[string]string {
	h := map[string]string{"Cookie": "admin_locale=" + core.PseudoLocale}
	for k, v := range extra {
		h[k] = v
	}
	return h
}

// sweepMainApp is a user admin with a boolean filter, two rows, a
// dashboard carrying every widget type, and a custom page, all in the
// pseudo-locale. It returns the data values the sweep must allow.
func sweepMainApp(t *testing.T) (*fiber.App, []string) {
	ua := newTestUserAdmin()
	ua.DeclaredFilters = []core.Filter{core.NewBooleanFilter("IsActive")}
	ua.createUser("a@example.com", true)
	ua.createUser("b@example.com", false)
	dashboard := &core.Dashboard{
		Title: "Overview",
		Widgets: []core.Widget{
			core.NewMetric("Users", func() any { return 2 }),
			core.NewStat("Signups", func() (any, float64) { return 7, 12.5 }),
			core.NewProgress("Onboarding", func() (float64, float64) { return 3, 10 }),
			core.NewTable("Recent", []string{"Email"}, func() []map[string]any { return []map[string]any{{"Email": "a@example.com"}} }),
			core.NewChart("Growth", func() []core.ChartPoint { return []core.ChartPoint{{Label: "Mon", Value: 5}} }),
			core.NewDonut("Plans", func() []core.ChartPoint { return []core.ChartPoint{{Label: "Pro", Value: 3}} }),
			core.NewActivity("Feed", func() []string { return []string{"user created"} }),
			core.NewTimeline("History", func() []core.TimelineEntry { return []core.TimelineEntry{{Title: "Launch", Time: "May"}} }),
		},
	}
	dir := t.TempDir()
	writeOverrideTemplate(t, dir, "pages/hello.html", `<p>{{t "Hello"}}</p>`)
	admin := core.New(core.WithModelAdmins(ua), core.WithDashboard(dashboard), core.WithPseudoLocale())
	admin.Route("/hello", PageHandler(func(pc *PageContext) error { return pc.Render("pages/hello.html", nil) }), core.WithPageLabel("Hello"))
	app := fiber.New()
	if err := Mount(app.Group("/admin"), admin, "/admin", WithTemplateDirs(dir)); err != nil {
		t.Fatalf("mount: %v", err)
	}
	allow := append([]string{"a@example.com", "b@example.com", "Mon", "Pro", "user created", "Launch", "May", "PO"}, localeNames...)
	return app, allow
}

func sweepInlineApp(t *testing.T) (*fiber.App, []string) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular, core.WithPseudoLocale())
	seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	return app, append([]string{"a@example.com", "Acme", "PO"}, localeNames...)
}

var sweepPages = []sweepPage{
	{area: "layout", name: "shell", path: "/admin/hello", app: sweepMainApp},
	{area: "list", name: "list", path: "/admin/users", app: sweepMainApp},
	{area: "list", name: "list fragment", path: "/admin/users?search=a", headers: map[string]string{"HX-Request": "true"}, app: sweepMainApp},
	{area: "forms", name: "create", path: "/admin/users/create", app: sweepMainApp},
	{area: "forms", name: "edit", path: "/admin/users/1/edit", app: sweepMainApp},
	{area: "forms", name: "inline edit", path: "/admin/organizations/1/edit", app: sweepInlineApp},
	{area: "detail", name: "detail", path: "/admin/users/1", app: sweepMainApp},
	{area: "detail", name: "delete", path: "/admin/users/1/delete", app: sweepMainApp},
	{area: "detail", name: "inline detail", path: "/admin/organizations/1", app: sweepInlineApp},
	{area: "detail", name: "dashboard", path: "/admin/", app: sweepMainApp},
	{area: "errors", name: "not found", path: "/admin/users/999", app: sweepMainApp},
}

func TestPseudoLocaleSweep(t *testing.T) {
	for _, p := range sweepPages {
		t.Run(p.area+"/"+p.name, func(t *testing.T) {
			if !sweepAreas[p.area] {
				t.Skipf("area %q not converted yet", p.area)
			}
			app, allow := p.app(t)
			resp := doGet(t, app, p.path, pseudoCookie(p.headers))
			if resp.StatusCode >= 500 {
				t.Fatalf("got %d", resp.StatusCode)
			}
			if left := untranslated(body(t, resp), allow...); len(left) > 0 {
				t.Errorf("untranslated on %s:\n  %s", p.path, strings.Join(left, "\n  "))
			}
		})
	}
}
