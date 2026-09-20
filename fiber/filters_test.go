package fiber

import (
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

// hostDomainFilter is written the way an application would write one:
// it implements core.Filter and nothing else, borrowing nothing from
// the framework's own filter types. Django calls this a
// SimpleListFilter -- lookups() plus queryset().
type hostDomainFilter struct{}

func (hostDomainFilter) Name() string  { return "Domain" }
func (hostDomainFilter) Label() string { return "Domain" }

func (hostDomainFilter) ChoicesWithLabels() [][2]string {
	return [][2]string{{"", "All"}, {"example.com", "example.com"}, {"other.test", "other.test"}}
}

func (hostDomainFilter) Apply(objects []any, raw string, modelAdmin core.ModelAdmin) []any {
	if raw == "" {
		return objects
	}
	field, ok := modelAdmin.Field("Email")
	if !ok {
		return objects
	}
	out := make([]any, 0, len(objects))
	for _, obj := range objects {
		email, _ := field.GetValue(obj).(string)
		if strings.HasSuffix(email, "@"+raw) {
			out = append(out, obj)
		}
	}
	return out
}

// TestAHostDefinedFilterNarrowsTheListAndItsExport: the whole point of
// the hook is that a host filter is not a second-class citizen -- it
// must ride in the same ListRequest as a built-in, so the export and
// delete_selected's "all N matching" agree with what the table shows.
func TestAHostDefinedFilterNarrowsTheListAndItsExport(t *testing.T) {
	ua := newTestUserAdmin()
	ua.DeclaredFilters = []core.Filter{hostDomainFilter{}}
	ua.createUser("keep@example.com", true)
	ua.createUser("drop@other.test", true)
	app := newTestApp(t, core.New(core.WithModelAdmins(ua)))

	page := body(t, doGet(t, app, "/admin/users?filter[Domain]=example.com", nil))
	if !strings.Contains(page, "keep@example.com") {
		t.Error("the host filter dropped a row it should have kept")
	}
	if strings.Contains(page, "drop@other.test") {
		t.Error("the host filter kept a row it should have dropped")
	}

	// The panel offers it like any other filter.
	if !strings.Contains(page, "other.test") {
		t.Error("the host filter's own choices are not in the panel")
	}

	// And the export narrows with it, which is the invariant that makes
	// a host filter equal to a built-in rather than cosmetic.
	csv := body(t, doGet(t, app, "/admin/users/export/csv?filter[Domain]=example.com", nil))
	if !strings.Contains(csv, "keep@example.com") || strings.Contains(csv, "drop@other.test") {
		t.Errorf("the export ignored the host filter:\n%s", csv)
	}
}

func rangeTestAdmin(t *testing.T) *testUserAdmin {
	t.Helper()
	admin := newTestUserAdmin()
	admin.DeclaredFilters = []core.Filter{core.NewDateFilter("Joined")}
	admin.DisplayFields = []string{"ID", "Email", "Joined"}
	admin.DeclaredFields = append(admin.DeclaredFields, core.NewField("Joined", core.FieldTypeDate))
	return admin
}

func TestTheDateFilterPanelOffersARangeForm(t *testing.T) {
	admin := rangeTestAdmin(t)
	admin.createUser("a@example.com", true)
	app := newTestApp(t, core.New(core.WithModelAdmins(admin)))

	page := body(t, doGet(t, app, "/admin/users?search=a&sort=-Email", nil))
	if !strings.Contains(page, `name="`+core.RangeFromField+`"`) {
		t.Fatal("no range form in the panel")
	}
	// The form must carry the rest of the list, or applying a range
	// silently drops the reader's search and sort.
	for _, want := range []string{`name="search" value="a"`, `name="sort" value="-Email"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the range form does not carry %s", want)
		}
	}
}

func TestARangeSubmissionNarrowsTheList(t *testing.T) {
	admin := rangeTestAdmin(t)
	inside := admin.createUser("inside@example.com", true)
	outside := admin.createUser("outside@example.com", true)
	inside.Joined = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	outside.Joined = time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	app := newTestApp(t, core.New(core.WithModelAdmins(admin)))

	// Exactly what the form submits, including the blank end.
	page := body(t, doGet(t, app, "/admin/users?"+core.RangeForField+"=Joined&"+
		core.RangeFromField+"=2026-01-01&"+core.RangeToField+"=", nil))
	if !strings.Contains(page, "inside@example.com") {
		t.Error("the range dropped a row inside it")
	}
	if strings.Contains(page, "outside@example.com") {
		t.Error("the range kept a row outside it")
	}
}

func TestThePanelPrefillsARangeItIsShowing(t *testing.T) {
	admin := rangeTestAdmin(t)
	app := newTestApp(t, core.New(core.WithModelAdmins(admin)))

	page := body(t, doGet(t, app, "/admin/users?filter[Joined]=2026-01-01:2026-03-01", nil))
	if !strings.Contains(page, `value="2026-01-01"`) || !strings.Contains(page, `value="2026-03-01"`) {
		t.Error("the panel did not prefill the range it is filtering by")
	}
}

func TestASetRangeCountsAsAnActiveFilter(t *testing.T) {
	// The badge says how much the panel is hiding without being opened,
	// and Clear-all has to drop a range like any other filter -- a range
	// that survived "Clear all" would be an invisible constraint.
	admin := rangeTestAdmin(t)
	admin.createUser("a@example.com", true)
	app := newTestApp(t, core.New(core.WithModelAdmins(admin)))

	page := body(t, doGet(t, app, "/admin/users?filter[Joined]=2026-01-01:2026-03-01", nil))
	count, err := uiClasses("filter-panel", "count")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(page, count) {
		t.Error("a set range is not counted in the Filters badge")
	}
	if !strings.Contains(page, "Clear all") {
		t.Error("no Clear-all control while a range is set")
	}
}

// TestTheNewFiltersNarrowTheExportToo: a filter that narrows the table
// but not the export turns "all N matching" into a lie, which is the one
// thing the single-ListRequest design exists to prevent.
func TestTheNewFiltersNarrowTheExportToo(t *testing.T) {
	admin := newTestUserAdmin()
	admin.DeclaredFilters = []core.Filter{
		core.NewEmptyFilter("Email"),
		core.NewDateFilter("Joined"),
	}
	admin.DisplayFields = []string{"ID", "Email", "Joined"}
	admin.DeclaredFields = append(admin.DeclaredFields, core.NewField("Joined", core.FieldTypeDate))
	blank := admin.createUser("", true)
	filled := admin.createUser("filled@example.com", true)
	blank.Joined = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	filled.Joined = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	app := newTestApp(t, core.New(core.WithModelAdmins(admin)))

	for _, c := range []struct{ name, query, want, notWant string }{
		{"empty", "filter[Email]=" + core.EmptyFilterNotEmpty, "filled@example.com", ""},
		{"range", "filter[Joined]=2026-01-01:2026-03-01", "filled@example.com", ""},
		{"range excludes", "filter[Joined]=2025-01-01:2025-03-01", "", "filled@example.com"},
	} {
		csv := body(t, doGet(t, app, "/admin/users/export/csv?"+c.query, nil))
		if c.want != "" && !strings.Contains(csv, c.want) {
			t.Errorf("%s: export dropped %q\n%s", c.name, c.want, csv)
		}
		if c.notWant != "" && strings.Contains(csv, c.notWant) {
			t.Errorf("%s: export kept %q\n%s", c.name, c.notWant, csv)
		}
	}
}
