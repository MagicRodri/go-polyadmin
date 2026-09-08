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
	scroll, err := uiClasses("table", "scroll")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(section, scroll) {
		t.Error("the inline table has no scroll container, so it clips instead of scrolling")
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

func TestManyToManyListboxInAnEditRowIsCapped(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	// More options than the cap, or there is nothing to cap.
	for i := 1; i <= inlineMultiSelectRows+4; i++ {
		orgAdmin.store[100+i] = &inlineTestOrg{ID: 100 + i, Name: "Team " + strconv.Itoa(i)}
	}

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)))
	if !strings.Contains(section, "<select multiple") {
		t.Fatal("no multi-select in the edit row; the assertions below would be vacuous")
	}
	if !strings.Contains(section, `size="`+strconv.Itoa(inlineMultiSelectRows)+`"`) {
		t.Errorf("the listbox is not capped at %d rows", inlineMultiSelectRows)
	}
	// It must not be sized to the option count.
	if strings.Contains(section, `size="`+strconv.Itoa(inlineMultiSelectRows+5)+`"`) {
		t.Error("the listbox is still sized to the number of options")
	}
}

func TestManyToManyListboxInAnEditRowUsesTheScrollAreaStyling(t *testing.T) {
	app, orgAdmin, userAdmin := newInlineTestApp(t, core.InlineLayoutTabular)
	org, _ := seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")
	orgAdmin.store[101] = &inlineTestOrg{ID: 101, Name: "Team"}

	section := inlineSection(t, body(t, doGet(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit", nil)))
	scroll, err := uiClasses("scroll-area", "y")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(section, scroll) {
		t.Error("the edit row's listbox does not carry the scroll-area classes")
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
