package fiber

import (
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestManyToManyCellInATabularInlineIsBounded(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, users := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	// The cell has to actually hold several relations, or the assertion
	// would pass against an empty one and prove nothing.
	users[0].Teams = []any{&inlineTestOrg{ID: 9, Name: "Platform"}, &inlineTestOrg{ID: 10, Name: "Security"}}

	page := body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil))
	section := inlineSection(t, page)
	if !strings.Contains(section, "Platform") {
		t.Fatal("the many-to-many cell rendered nothing; the assertion below would be vacuous")
	}
	scroll, err := uiClasses("scroll-area", "x")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(section, scroll) {
		t.Error("the many-to-many cell is not wrapped in a scroll area")
	}
}

func TestOnlyManyToManyCellsAreBounded(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	page := body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil))
	section := inlineSection(t, page)
	scroll, _ := uiClasses("scroll-area", "x")
	// This fixture's child has one many-to-many-shaped column at most;
	// counting pins that the wrapper is per-field, not per-cell.
	if got := strings.Count(section, scroll); got > 1 {
		t.Errorf("%d cells were bounded; only the many-to-many should be", got)
	}
}

func TestTabularInlineTableCanScrollRatherThanClip(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil)))
	scroll, err := uiClasses("table", "inline-scroll")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(section, scroll) {
		t.Error("the inline table has no scroll container, so it clips instead of scrolling")
	}
}

// TestALongTabularInlineScrollsInsideItsOwnCard: unlike the top-level
// list table (which is meant to grow the page), a tabular inline sits
// inside a parent's detail/edit page -- many child rows should scroll
// in place, not stretch the page, same reasoning as the widget and
// stacked-inline cards.
func TestALongTabularInlineScrollsInsideItsOwnCard(t *testing.T) {
	emails := make([]string, 50)
	for i := range emails {
		emails[i] = "user" + strconv.Itoa(i) + "@example.com"
	}
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, emails...)

	inlineScroll, err := uiClasses("table", "inline-scroll")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil)))
	if !strings.Contains(section, "user49@example.com") {
		t.Fatal("the tabular inline did not render every row; the assertion below would be vacuous")
	}
	if !strings.Contains(inlineScroll, "max-h-") || !strings.Contains(inlineScroll, "overflow-y-auto") {
		t.Errorf("the inline table is not bounded and scrollable: %q", inlineScroll)
	}
	if !strings.Contains(inlineScroll, "ui-scroll-area") {
		t.Errorf("the inline table does not use the themed scrollbar: %q", inlineScroll)
	}
	// The main list table must keep the page-level scroll it already had.
	listScroll, _ := uiClasses("table", "scroll")
	if strings.Contains(listScroll, "max-h-") {
		t.Errorf("the list table's own scroll part got bounded too: %q", listScroll)
	}
}

// TestScrollAreaContainsOverscrollRatherThanChainingToThePage: without
// overscroll-behavior, wheel input that outruns a bounded box's own
// scroll range chains into whichever ancestor scrolls next -- on the
// edit page that is the whole document, so scrolling to the bottom of a
// tabular inline suddenly yanks the page too. Verified live with
// Playwright: scrollY stayed 0 with `overscroll-behavior: contain` set,
// and jumped to 1200 without it.
func TestScrollAreaContainsOverscrollRatherThanChainingToThePage(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users", nil))
	idx := strings.Index(page, ".ui-scroll-area {")
	if idx < 0 {
		t.Fatal("no .ui-scroll-area rule on the page; the assertion below would be vacuous")
	}
	rule := page[idx : idx+500]
	if !strings.Contains(rule, "overscroll-behavior: contain") {
		t.Errorf("the scroll area does not contain overscroll, so scrolling past its own edge leaks into the page: %q", rule)
	}
}

func TestListTableUsesTheRegistrysScrollPart(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users", nil))
	scroll, _ := uiClasses("table", "scroll")
	if !strings.Contains(page, scroll) {
		t.Error("the list table does not use ui(\"table\", \"scroll\")")
	}
}

// inlineSection returns just the inline region, bounded at the page's
// own action bar -- the parent's controls render further down the same
// page and would otherwise be counted as part of the section.
func inlineSection(t *testing.T, page string) string {
	t.Helper()
	parts := strings.Split(page, `id="inline-users"`)
	if len(parts) < 2 {
		t.Fatal("no inline section on the page")
	}
	section := parts[1]
	pageActions, err := uiClasses("page", "actions")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if idx := strings.Index(section, pageActions); idx >= 0 {
		section = section[:idx]
	}
	return section
}

// TestManyToManyInAnEditRowIsTheShadcnControl: the row holds the same
// trigger-and-popover control the full form uses, not a native
// <select multiple> sized to its options.
func TestManyToManyInAnEditRowIsTheShadcnControl(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	for i := 1; i <= inlineMultiSelectRows+4; i++ {
		orgAdmin.store[100+i] = &inlineTestOrg{ID: 100 + i, Name: "Team " + strconv.Itoa(i)}
	}

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)))
	if strings.Contains(section, "<select multiple") {
		t.Error("the native listbox is still there")
	}
	if !strings.Contains(section, `aria-haspopup="listbox"`) {
		t.Fatal("no shadcn multi-select in the edit row")
	}
	// Its popover leaves the row rather than being clipped by it.
	if !strings.Contains(section, `x-teleport="body"`) {
		t.Error("the popover is not portalled out of the row")
	}
	// Nothing is sized to the option count any more.
	if strings.Contains(section, `size="`+strconv.Itoa(inlineMultiSelectRows)+`"`) {
		t.Error("something in the row is still sized in rows")
	}
}

func TestARelationCellInAnEditRowIsTheShadcnSelect(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)))
	if !strings.Contains(section, "adminMultiSelect()") && !strings.Contains(section, "adminSelect()") {
		t.Error("the row's relation control is not one of the admin's own")
	}
}

func TestPageWithATabularInlineUsesTheWideBody(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	wide, err := uiClasses("page", "body-wide")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	for _, path := range []string{"", "/edit"} {
		page := body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+path, nil))
		if !strings.Contains(page, wide) {
			t.Errorf("%q does not use the wide body", path)
		}
		// The action bar has to widen with it or the buttons drift out
		// of line with the card above them.
		actionsWide, _ := uiClasses("page", "actions-inner-wide")
		if !strings.Contains(page, actionsWide) {
			t.Errorf("%q widened the card but not its action bar", path)
		}
	}
}

func TestAPlainFormKeepsTheNarrowBody(t *testing.T) {
	app, userAdmin := makeApp(t)
	// Seeded, or the route 404s and the page under test is the error
	// page rather than a form.
	userAdmin.store[1] = &testUser{ID: 1, Email: "a@example.com"}
	page := body(t, doGet(t, app, "/admin/users/1/edit", nil))
	if !strings.Contains(page, "resource-form") {
		t.Fatal("not a form page; the assertions below would be vacuous")
	}
	narrow, err := uiClasses("page", "body")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	wide, _ := uiClasses("page", "body-wide")
	if !strings.Contains(page, narrow) {
		t.Error("a plain form lost the narrow body")
	}
	if strings.Contains(page, wide) {
		t.Error("a plain form was widened; only a tabular inline should do that")
	}
}

func TestAStackedInlineKeepsTheNarrowBody(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutStacked)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	page := body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil))
	wide, _ := uiClasses("page", "body-wide")
	if strings.Contains(page, wide) {
		t.Error("a stacked inline widened the page")
	}
}

// TestAStackedInlineRecordScrollsInsideItsOwnCard: a related record with
// many fields otherwise grows its card to fit them, same problem as a
// long dashboard widget. The body is bounded instead, and scrolls with
// the themed scrollbar -- see TestALongWidgetScrollsInsideItsOwnCard.
func TestAStackedInlineRecordScrollsInsideItsOwnCard(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutStacked)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	panelBody, err := uiClasses("panel", "body")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID), nil)))
	if !strings.Contains(section, "a@example.com") {
		t.Fatal("the related record rendered nothing; the assertion below would be vacuous")
	}
	if !strings.Contains(section, panelBody) {
		t.Fatal("the related record's card is not the bounded scroll box")
	}
	if !strings.Contains(panelBody, "max-h-") || !strings.Contains(panelBody, "overflow-y-auto") {
		t.Errorf("the card body is not bounded and scrollable: %q", panelBody)
	}
	if !strings.Contains(panelBody, "ui-scroll-area") {
		t.Errorf("the card body does not use the themed scrollbar: %q", panelBody)
	}
}

func TestSelectInATableCellSizesToItsContent(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)))
	cell, err := uiClasses("select", "cell-multi")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(section, cell) {
		t.Error("the cell's select does not use the content-sized part")
	}
	// The w-full base would defeat the whole point.
	base, _ := uiClasses("select", "size-sm")
	if strings.Contains(section, base) {
		t.Error("a cell select is still using the w-full base")
	}
}
