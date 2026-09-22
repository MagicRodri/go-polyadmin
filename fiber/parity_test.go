package fiber

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// newParityApp mounts one users admin the caller has already configured.
func newParityApp(t *testing.T, users *testUserAdmin) *fiber.App {
	t.Helper()
	return newTestApp(t, core.New(core.WithModelAdmins(users)))
}

func TestANonSortableColumnHasNoSortMenu(t *testing.T) {
	users := newTestUserAdmin()
	users.SortableFieldNames = []string{"Email"}
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users", nil))

	if !strings.Contains(page, "sort=Email") {
		t.Error("the sortable column lost its sort links")
	}
	if strings.Contains(page, "sort=IsActive") || strings.Contains(page, "sort=-IsActive") {
		t.Error("a column outside sortable_by still offers a sort")
	}
}

func TestEverySortMenuSurvivesWhenSortableIsUnset(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users", nil))
	for _, want := range []string{"sort=Email", "sort=IsActive", "sort=ID"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestTheFirstCellLinksToTheRecordByDefault(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users", nil))
	// The cell links to the record, carrying the list it came from.
	if !strings.Contains(page, `<a href="/admin/users/1?_list=`) {
		t.Errorf("the first cell does not link to the record:\n%s", page)
	}
	// The link belongs to the ID cell, not every cell: one cell link plus
	// the row menu's View.
	if got := strings.Count(page, `href="/admin/users/1?_list=`); got != 2 {
		t.Errorf("expected exactly one linked cell, found %d links", got)
	}
}

func TestListDisplayLinksMovesTheLinkAndEmptyRemovesIt(t *testing.T) {
	users := newTestUserAdmin()
	users.LinkFieldNames = []string{"Email"}
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users", nil))
	if !strings.Contains(page, `<a href="/admin/users/1?_list=`) || !strings.Contains(page, `>a@example.com</a>`) {
		t.Error("the named cell does not link")
	}

	none := newTestUserAdmin()
	none.LinkFieldNames = []string{}
	none.createUser("a@example.com", true)
	page = body(t, doGet(t, newParityApp(t, none), "/admin/users", nil))
	if got := strings.Count(page, `href="/admin/users/1?_list=`); got != 1 { // only the row menu's View
		t.Errorf("an empty list_display_links still linked a cell: %d links", got)
	}
}

// relationLinkAdmin's first list column renders a relation, whose cell is
// already an anchor.
func TestACellThatIsAlreadyALinkIsNotWrappedAgain(t *testing.T) {
	orgAdmin := newInlineTestOrgAdmin(core.InlineLayoutTabular)
	userAdmin := newInlineTestUserAdmin(orgAdmin)
	userAdmin.DisplayFields = []string{"Organization", "Email"}
	userAdmin.LinkFieldNames = []string{"Organization"}
	admin := core.New(core.WithModelAdmins(orgAdmin, userAdmin))
	app := newTestApp(t, admin)
	seedOrgWithUsers(orgAdmin, userAdmin, "a@example.com")

	page := body(t, doGet(t, app, "/admin/users", nil))
	if strings.Contains(page, `class="text-primary underline-offset-4 hover:underline"><a`) {
		t.Error("a relation cell was wrapped in a second anchor")
	}
	// The relation's own link survives.
	if !strings.Contains(page, `href="/admin/organizations/1"`) {
		t.Error("the relation cell lost its own link")
	}
}

// filteredList is the list URL the tests navigate away from and expect to
// come back to.
const filteredList = "/admin/users?search=a&sort=-Email"

func TestListLinksCarryTheListTheyCameFrom(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), filteredList, nil))
	// Every way out of the list carries it: the row's View/Edit/Delete
	// and the New button.
	for _, want := range []string{
		`/admin/users/1?_list=`,
		`/admin/users/1/edit?_list=`,
		`/admin/users/1/delete?_list=`,
		`/admin/users/create?_list=`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !strings.Contains(page, "search%3Da") {
		t.Error("the token does not carry the search")
	}
}

func TestDisablingPreserveFiltersDropsTheToken(t *testing.T) {
	users := newTestUserAdmin()
	users.DisablePreserveFilters = true
	users.createUser("a@example.com", true)
	page := body(t, doGet(t, newParityApp(t, users), filteredList, nil))
	if strings.Contains(page, "_list=") {
		t.Error("a disabled preserve_filters still emitted the token")
	}
}

func TestTheBreadcrumbLeadsBackIntoTheFilteredList(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	token := url.QueryEscape(filteredList)
	for _, path := range []string{
		"/admin/users/1?_list=" + token,
		"/admin/users/1/edit?_list=" + token,
		"/admin/users/1/delete?_list=" + token,
	} {
		page := body(t, doGet(t, app, path, nil))
		if !strings.Contains(page, `href="/admin/users?search=a&amp;sort=-Email"`) {
			t.Errorf("%s: the breadcrumb lost the list it came from", path)
		}
	}
}

func TestSavingReturnsToTheListItCameFrom(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	token := url.QueryEscape(filteredList)

	// The edit page's form carries the token, and the save keeps it on the
	// record's own page, whose breadcrumb then leads back to the list.
	page := body(t, doGet(t, app, "/admin/users/1/edit?_list="+token, nil))
	if !strings.Contains(page, `name="_list" value="/admin/users?search=a&amp;sort=-Email"`) {
		t.Fatalf("the edit form does not carry the token:\n%s", page)
	}
	resp := doPostForm(t, app, "/admin/users/1/edit", url.Values{
		"Email": {"a@example.com"}, "IsActive": {"on"}, "_list": {filteredList},
	}, nil)
	if got := resp.Header.Get("Location"); got != "/admin/users/1?_list="+token {
		t.Errorf("Location = %q", got)
	}
}

func TestSaveAndAddAnotherKeepsTheList(t *testing.T) {
	users := newTestUserAdmin()
	app := newParityApp(t, users)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{
		"Email": {"b@example.com"}, "IsActive": {"on"},
		"_addanother": {"1"}, "_list": {filteredList},
	}, nil)
	if got := resp.Header.Get("Location"); got != "/admin/users/create?_list="+url.QueryEscape(filteredList) {
		t.Errorf("Location = %q", got)
	}
}

func TestDeletingReturnsToTheFilteredList(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	resp := doPostForm(t, app, "/admin/users/1/delete", url.Values{"_list": {filteredList}}, nil)
	if got := resp.Header.Get("Location"); got != filteredList {
		t.Errorf("Location = %q, want the list it came from", got)
	}
}

func TestAnOffsiteListTokenIsDiscarded(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	resp := doPostForm(t, app, "/admin/users/1/delete",
		url.Values{"_list": {"https://evil.example/admin/users"}}, nil)
	if got := resp.Header.Get("Location"); got != "/admin/users" {
		t.Errorf("Location = %q, want the bare list", got)
	}
}

func TestSaveAsNewAppearsOnlyOnTheEditFormAndOnlyWhenAllowed(t *testing.T) {
	users := newTestUserAdmin()
	users.AllowSaveAs = true
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	if !strings.Contains(body(t, doGet(t, app, "/admin/users/1/edit", nil)), `name="_saveasnew"`) {
		t.Error("the edit form is missing Save as new")
	}
	if strings.Contains(body(t, doGet(t, app, "/admin/users/create", nil)), `name="_saveasnew"`) {
		t.Error("the create form offers Save as new")
	}

	off := newTestUserAdmin()
	off.createUser("a@example.com", true)
	if strings.Contains(body(t, doGet(t, newParityApp(t, off), "/admin/users/1/edit", nil)), `name="_saveasnew"`) {
		t.Error("save_as is off but the button is there")
	}
}

func TestSaveAsNewCreatesACopyAndLeavesTheOriginal(t *testing.T) {
	users := newTestUserAdmin()
	users.AllowSaveAs = true
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)

	resp := doPostForm(t, app, "/admin/users/1/edit", url.Values{
		"Email": {"copy@example.com"}, "IsActive": {"on"}, "_saveasnew": {"1"},
	}, nil)
	if len(users.store) != 2 {
		t.Fatalf("expected a second record, have %d", len(users.store))
	}
	if users.store[1].Email != "a@example.com" {
		t.Errorf("the original was modified: %q", users.store[1].Email)
	}
	if users.store[2].Email != "copy@example.com" {
		t.Errorf("the copy holds %q", users.store[2].Email)
	}
	// The redirect lands on the new record, which is where a create lands.
	if got := resp.Header.Get("Location"); got != "/admin/users/2" {
		t.Errorf("Location = %q", got)
	}
}

func TestSaveAsNewIsAnOrdinarySaveWhenTheOptionIsOff(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	doPostForm(t, app, "/admin/users/1/edit", url.Values{
		"Email": {"renamed@example.com"}, "IsActive": {"on"}, "_saveasnew": {"1"},
	}, nil)
	if len(users.store) != 1 || users.store[1].Email != "renamed@example.com" {
		t.Errorf("a forged _saveasnew was honoured: %d records", len(users.store))
	}
}

func TestSaveAsNewRedisplaysTheFormOnAValidationError(t *testing.T) {
	users := newTestUserAdmin()
	users.AllowSaveAs = true
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)
	resp := doPostForm(t, app, "/admin/users/1/edit", url.Values{
		"Email": {""}, "IsActive": {"on"}, "_saveasnew": {"1"},
	}, nil)
	if resp.StatusCode != fiber.StatusUnprocessableEntity || len(users.store) != 1 {
		t.Errorf("got %d with %d records", resp.StatusCode, len(users.store))
	}
}

func TestPrepopulatedFieldsRideOnTheCreateFormOnly(t *testing.T) {
	users := newTestUserAdmin()
	users.PrepopulatedFields = map[string][]string{"Email": {"Email"}}
	users.createUser("a@example.com", true)
	app := newParityApp(t, users)

	create := body(t, doGet(t, app, "/admin/users/create", nil))
	if !strings.Contains(create, "data-prepopulated=") || !strings.Contains(create, "Email") {
		t.Error("the create form does not carry the prepopulation map")
	}
	// An existing record's slug is a real identifier; it is never rewritten.
	if strings.Contains(body(t, doGet(t, app, "/admin/users/1/edit", nil)), "data-prepopulated=") {
		t.Error("the edit form carries the prepopulation map")
	}
}

func TestNoPrepopulationAttributeWithoutTheOption(t *testing.T) {
	users := newTestUserAdmin()
	if strings.Contains(body(t, doGet(t, newParityApp(t, users), "/admin/users/create", nil)), "data-prepopulated=") {
		t.Error("an admin without prepopulated_fields still emitted the attribute")
	}
}

func TestPrepopulatedUnicodeFieldsAreMarked(t *testing.T) {
	users := newTestUserAdmin()
	users.PrepopulatedFields = map[string][]string{"Email": {"Email"}}
	users.PrepopulatedUnicode = []string{"Email"}
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users/create", nil))
	if !strings.Contains(page, "unicode") {
		t.Errorf("the unicode opt-out is not in the attribute:\n%s", page)
	}
}

// newDatedUserAdmin declares a DateFilter over the users admin's own date
// field, the way an application would.
func newDatedUserAdmin() *testUserAdmin {
	users := newTestUserAdmin()
	users.DisplayFields = []string{"ID", "Email", "Joined"}
	users.DeclaredFields = append(users.DeclaredFields, core.NewField("Joined", core.FieldTypeDate))
	users.DeclaredFilters = []core.Filter{core.NewDateFilter("Joined")}
	return users
}

func TestTheDateFilterRendersInTheFilterPanel(t *testing.T) {
	users := newDatedUserAdmin()
	users.createUser("a@example.com", true).Joined = time.Now()
	page := body(t, doGet(t, newParityApp(t, users), "/admin/users", nil))

	// One entry in the panel, with the presets as its choices -- exactly
	// how a boolean or choice filter renders.
	for _, want := range []string{"Joined", "Any date", "Today", "Past 7 days", "This month", "This year"} {
		if !strings.Contains(page, want) {
			t.Errorf("missing %q from the filter panel", want)
		}
	}
	if !strings.Contains(page, "filter[Joined]=today") {
		t.Error("the preset has no URL to click")
	}
	// And nothing above the table.
	if strings.Contains(page, "All dates") {
		t.Error("the old drill-down strip is still rendered")
	}
}

func TestTheDateFilterNarrowsTheRows(t *testing.T) {
	users := newDatedUserAdmin()
	users.createUser("recent@example.com", true).Joined = time.Now()
	users.createUser("old@example.com", true).Joined = time.Now().AddDate(-2, 0, 0)

	page := body(t, doGet(t, newParityApp(t, users), "/admin/users?filter[Joined]=year", nil))
	if !strings.Contains(page, "recent@example.com") || strings.Contains(page, "old@example.com") {
		t.Error("the date filter did not reach the rows")
	}
}

func TestNoDateFilterUnlessDeclared(t *testing.T) {
	users := newTestUserAdmin()
	users.createUser("a@example.com", true)
	if strings.Contains(body(t, doGet(t, newParityApp(t, users), "/admin/users", nil)), "Any date") {
		t.Error("an admin that declares no date filter still shows one")
	}
}
