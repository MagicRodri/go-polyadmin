package fiber

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

// renderedDashboard is a convenient full page to assert layout-level
// concerns against -- the dashboard needs no fixtures.
func renderedDashboard(t *testing.T) string {
	t.Helper()
	admin := core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithDashboard(&core.Dashboard{}))
	app := newTestApp(t, admin)
	return body(t, doGet(t, app, "/admin/", nil))
}

func TestLayoutEmitsThemeTokens(t *testing.T) {
	page := renderedDashboard(t)
	// The light palette on :root and the dark override, both as bare HSL
	// triplets so Tailwind's <alpha-value> opacity modifiers work.
	for _, want := range []string{
		"--background: 0 0% 100%;",
		"--muted-foreground: 240 3.8% 46.1%;",
		"--radius: 0.5rem;",
		// Chart tokens are for *categorical data* (the Donut widget's
		// slices) as opposed to UI chrome -- the same split shadcn
		// draws. Without them a Donut would fall back to literal
		// Tailwind shades and stop following the theme.
		"--chart-1:",
		"--chart-6:",
		".dark {",
		"--background: 240 10% 3.9%;",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("theme tokens missing %q", want)
		}
	}
}

func TestLayoutConfiguresTailwindForTokensAndDarkMode(t *testing.T) {
	page := renderedDashboard(t)
	for _, want := range []string{
		`darkMode: "class"`,
		"hsl(var(--border) / <alpha-value>)",
		"hsl(var(--primary) / <alpha-value>)",
		"hsl(var(--chart-1) / <alpha-value>)",
		"var(--radius)",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("tailwind config missing %q", want)
		}
	}
	// Order matters, and it's the order that isn't obvious: the CDN
	// script has to load *before* tailwind.config is assigned, since
	// loading it is what defines the `tailwind` global in the first
	// place. Assigning to it first throws a ReferenceError the browser
	// swallows silently -- confirmed with a real headless-Chrome run,
	// where getting this backwards left tailwind.config as {} and not
	// one token-based utility (.bg-background, .text-chart-1, ...)
	// existed in the generated stylesheet.
	cdnAt := strings.Index(page, "cdn.tailwindcss.com")
	configAt := strings.Index(page, "tailwind.config")
	if configAt < 0 || cdnAt < 0 || cdnAt > configAt {
		t.Errorf("the CDN script (at %d) must precede tailwind.config (at %d)", cdnAt, configAt)
	}
}

func TestLayoutResolvesThemeBeforePaint(t *testing.T) {
	page := renderedDashboard(t)
	// The class has to be set by a synchronous inline script before the
	// first paint, or a dark-mode reload flashes the light palette.
	if !strings.Contains(page, `localStorage.getItem("polyadmin-theme")`) {
		t.Error("missing the pre-paint theme resolution script")
	}
	if !strings.Contains(page, `classList.toggle("dark", dark)`) {
		t.Error("missing the dark-class toggle")
	}
	scriptAt := strings.Index(page, `localStorage.getItem("polyadmin-theme")`)
	bodyAt := strings.Index(page, "<body")
	if scriptAt < 0 || bodyAt < 0 || scriptAt > bodyAt {
		t.Errorf("the pre-paint script (at %d) must come before <body> (at %d)", scriptAt, bodyAt)
	}
}

func TestLayoutLoadsAlpinePluginsBeforeAlpineCore(t *testing.T) {
	page := renderedDashboard(t)
	// x-trap (the confirm dialog), x-collapse (the sidebar accordion),
	// and x-anchor (dropdown/date-picker popovers) all come from
	// plugins, and Alpine only registers directives that exist by the
	// time core initializes -- so plugins must load first.
	coreAt := strings.Index(page, "unpkg.com/alpinejs@")
	if coreAt < 0 {
		t.Fatal("Alpine core is not loaded")
	}
	for _, plugin := range []string{"@alpinejs/focus", "@alpinejs/collapse", "@alpinejs/anchor"} {
		at := strings.Index(page, plugin)
		if at < 0 {
			t.Errorf("%s is not loaded", plugin)
			continue
		}
		if at > coreAt {
			t.Errorf("%s (at %d) must load before Alpine core (at %d)", plugin, at, coreAt)
		}
	}
}

func TestLayoutUsesTokenClassesNotLiteralPalette(t *testing.T) {
	page := renderedDashboard(t)
	if !strings.Contains(page, "bg-background") || !strings.Contains(page, "text-foreground") {
		t.Error("expected the body to be painted with theme tokens")
	}
	// The <style>/<script> blocks legitimately mention the palette-free
	// HSL numbers, but no *class* should name a literal Tailwind shade.
	for _, banned := range []string{"bg-neutral-", "text-neutral-", "border-neutral-", "bg-gray-"} {
		if strings.Contains(page, banned) {
			t.Errorf("rendered page still uses the literal palette %q", banned)
		}
	}
}

func TestLayoutRendersThemeToggle(t *testing.T) {
	page := renderedDashboard(t)
	if !strings.Contains(page, "$store.theme.toggle()") {
		t.Error("missing the theme toggle button")
	}
	if !strings.Contains(page, `Alpine.store("theme"`) {
		t.Error("missing the theme store the toggle reads")
	}
	if !strings.Contains(page, `aria-label="Toggle dark mode"`) {
		t.Error("theme toggle has no accessible name")
	}
}

func TestDonutSlicesUseChartTokens(t *testing.T) {
	// The Donut's palette lives in core (core/widget.go's donutColors)
	// but must resolve through the theme, so a dashboard's categorical
	// colors follow a retheme and get dark-mode-tuned values.
	dashboard := &core.Dashboard{
		Widgets: []core.Widget{
			core.NewDonut("Devices", func() []core.ChartPoint {
				return []core.ChartPoint{{Label: "Desktop", Value: 60}, {Label: "Mobile", Value: 40}}
			}),
		},
	}
	admin := core.New(core.WithModelAdmins(newTestUserAdmin()), core.WithDashboard(dashboard))
	app := newTestApp(t, admin)
	page := body(t, doGet(t, app, "/admin/", nil))

	if !strings.Contains(page, "bg-chart-1") || !strings.Contains(page, "text-chart-1") {
		t.Error("expected Donut slices to be painted with chart tokens")
	}
	for _, banned := range []string{"bg-blue-500", "text-blue-500", "bg-violet-500"} {
		if strings.Contains(page, banned) {
			t.Errorf("Donut still uses the literal palette %q", banned)
		}
	}
}
