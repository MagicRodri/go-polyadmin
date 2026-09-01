package fiber

import (
	"io/fs"
	"sort"
	"strings"
	"testing"

	coretemplates "github.com/MagicRodri/go-polyadmin/templates"
)

// The two implementations' template trees are kept path-for-path
// identical, so a file that does a job in one repository has the same
// name and location in the other (docs/templates.md). Pinned as an
// explicit list rather than read from the Python tree -- the repos are
// separate checkouts, and the point is to fail when someone moves or
// adds a template on one side only.
//
// The mirror of this test lives in python-polyadmin/tests/test_layout.py.
func TestTemplateTreeMatchesThePythonImplementationPathForPath(t *testing.T) {
	expected := map[string]bool{
		"admin/base.html":      true,
		"admin/theme.html":     true,
		"admin/login.html":     true,
		"admin/dashboard.html": true,

		"admin/resource/list.html":   true,
		"admin/resource/detail.html": true,
		"admin/resource/form.html":   true,
		"admin/resource/delete.html": true,

		"admin/components/list_content.html":         true,
		"admin/components/search.html":               true,
		"admin/components/form_wrapper.html":         true,
		"admin/components/inline.html":               true,
		"admin/components/inline_fragment.html":      true,
		"admin/components/lookup_results.html":       true,
		"admin/components/toasts.html":               true,
		"admin/components/action_confirm_modal.html": true,
		"admin/components/csrf-field.html":           true,

		"admin/components/ui/breadcrumb.html":    true,
		"admin/components/ui/bulk-actions.html":  true,
		"admin/components/ui/calendar.html":      true,
		"admin/components/ui/dropdown-menu.html": true,
		"admin/components/ui/field.html":         true,
		"admin/components/ui/filter-panel.html":  true,
		"admin/components/ui/multi-select.html":  true,
		"admin/components/ui/pagination.html":    true,
		"admin/components/ui/radio-group.html":   true,
		"admin/components/ui/select.html":        true,
		"admin/components/ui/sidebar.html":       true,
		"admin/components/ui/slider.html":        true,
		"admin/components/ui/switch.html":        true,
		"admin/components/ui/table.html":         true,
		"admin/components/ui/theme-toggle.html":  true,

		"admin/widgets/activity.html": true,
		"admin/widgets/chart.html":    true,
		"admin/widgets/donut.html":    true,
		"admin/widgets/metric.html":   true,
		"admin/widgets/progress.html": true,
		"admin/widgets/stat.html":     true,
		"admin/widgets/table.html":    true,
		"admin/widgets/tabs.html":     true,
		"admin/widgets/timeline.html": true,
	}

	var found []string
	err := fs.WalkDir(coretemplates.FS, "admin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".html") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	sort.Strings(found)

	for _, path := range found {
		if !expected[path] {
			t.Errorf("template %q exists but is not in the expected layout (add it to the Python tree at the same path, and to this list)", path)
		}
	}
	for path := range expected {
		if _, statErr := fs.Stat(coretemplates.FS, path); statErr != nil {
			t.Errorf("template %q is expected by the shared layout but missing here", path)
		}
	}
}

// Python has two templates Go does not, because Go builds the same HTML
// in Go code -- see the fiber package doc comment on why field and form
// markup is assembled there rather than in html/template. Pinned so the
// exception stays a deliberate two-file list rather than growing
// quietly.
func TestPythonOnlyTemplatesAreNotAccidentallyAddedHere(t *testing.T) {
	for _, path := range []string{
		"admin/components/icons.html", // Go: fiber/icons.go
		"admin/components/field.html", // Go: fiber/render_helpers.go
	} {
		if _, err := fs.Stat(coretemplates.FS, path); err == nil {
			t.Errorf("%q now exists in Go; either the Go-code equivalent is dead, or this exception list is stale", path)
		}
	}
}
