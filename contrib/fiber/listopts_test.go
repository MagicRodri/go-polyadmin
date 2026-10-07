package fiber

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func newPagedAdmin(rows, perPage int) *testUserAdmin {
	a := newTestUserAdmin()
	a.PageSizeDefault = perPage
	// A stable order, or "the first page" is not a well-defined set.
	a.OrderingDefault = "Email"
	a.store = map[int]*testUser{}
	for i := 1; i <= rows; i++ {
		a.store[i] = &testUser{ID: i, Email: "u" + strconv.Itoa(1000+i) + "@example.com", IsActive: true}
	}
	return a
}

func countRows(page string) int {
	// One <tr> per record plus the header row.
	return strings.Count(page, "<tr") - 1
}

func TestListPerPageIsHonouredWhenTheRequestNamesNoSize(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newPagedAdmin(30, 5)))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil))
	if got := countRows(page); got != 5 {
		t.Errorf("got %d rows, want the ModelAdmin's PageSizeDefault of 5", got)
	}
}

func TestListPerPageBeatsTheFrameworkDefault(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newPagedAdmin(30, 5)))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil))
	if countRows(page) == core.DefaultPageSize {
		t.Error("fell back to the framework default instead of the ModelAdmin's")
	}
}

func TestPagerReflectsTheModelAdminsPageSize(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newPagedAdmin(30, 5)))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil))
	// Matched against the pager's own sentence (ui/pagination.html), not
	// a bare "6" that could come from anywhere on the page: 30 rows at 5
	// per page is 6 pages, where the old unresolved request would have
	// produced 2.
	pager := regexp.MustCompile(`Page\s+1\s+of\s+(\d+)`).FindStringSubmatch(page)
	if pager == nil {
		t.Fatal("no pager on the page")
	}
	if pager[1] != "6" {
		t.Errorf("pager shows %s pages, want 6 -- it was sized from the unresolved request", pager[1])
	}
}

func TestExplicitPageSizeBeatsTheModelAdminsDefault(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newPagedAdmin(30, 5)))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users?page_size=10", nil))
	if got := countRows(page); got != 10 {
		t.Errorf("got %d rows, want the requested 10", got)
	}
}

func TestWithoutAPageSizeDefaultTheFrameworkDefaultApplies(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newPagedAdmin(30, 0)))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users", nil))
	if got := countRows(page); got != core.DefaultPageSize {
		t.Errorf("got %d rows, want the framework default %d", got, core.DefaultPageSize)
	}
}

func TestBlankValueShowsThePlaceholder(t *testing.T) {
	a := newTestUserAdmin()
	a.store = map[int]*testUser{1: {ID: 1, Email: ""}}
	admin := core.New(core.WithModelAdmins(a))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users/1", nil))
	if !strings.Contains(page, core.DefaultEmptyValue) {
		t.Error("a blank value rendered as nothing at all")
	}
}

func TestEmptyValueDisplayIsConfigurable(t *testing.T) {
	a := newTestUserAdmin()
	a.EmptyValueDisplay = "not set"
	a.store = map[int]*testUser{1: {ID: 1, Email: ""}}
	admin := core.New(core.WithModelAdmins(a))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users/1", nil))
	if !strings.Contains(page, "not set") {
		t.Error("EmptyValueDisplay was ignored")
	}
}

func TestZeroAndFalseAreNotTreatedAsEmpty(t *testing.T) {
	a := newTestUserAdmin()
	a.store = map[int]*testUser{1: {ID: 0, Email: "someone@example.com", IsActive: false}}
	admin := core.New(core.WithModelAdmins(a))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users/0", nil))
	if strings.Contains(page, core.DefaultEmptyValue) {
		t.Error("a zero int or false bool was rendered as empty")
	}
}

func TestEmptyValueDisplayIsEscaped(t *testing.T) {
	a := newTestUserAdmin()
	a.EmptyValueDisplay = `<script>alert(1)</script>`
	a.store = map[int]*testUser{1: {ID: 1, Email: ""}}
	admin := core.New(core.WithModelAdmins(a))
	page := body(t, doGet(t, newTestApp(t, admin), "/admin/users/1", nil))
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Error("EmptyValueDisplay was injected as raw markup")
	}
}

func TestSaveAndAddAnotherReturnsToAnEmptyForm(t *testing.T) {
	app, _ := makeApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{
		"Email":       {"first@example.com"},
		"_addanother": {"1"},
	}, nil)
	if got := resp.Header.Get("Location"); !strings.HasSuffix(got, "/admin/users/create") {
		t.Errorf("Location = %q, want the empty create form", got)
	}
}

func TestSaveAndAddAnotherStillCreatesTheRecord(t *testing.T) {
	app, userAdmin := makeApp(t)
	doPostForm(t, app, "/admin/users/create", url.Values{
		"Email":       {"first@example.com"},
		"_addanother": {"1"},
	}, nil)
	var found bool
	for _, u := range userAdmin.store {
		if u.Email == "first@example.com" {
			found = true
		}
	}
	if !found {
		t.Error("the record was not created")
	}
}

func TestPlainSaveStillGoesToTheRecord(t *testing.T) {
	app, _ := makeApp(t)
	resp := doPostForm(t, app, "/admin/users/create", url.Values{"Email": {"first@example.com"}}, nil)
	location := resp.Header.Get("Location")
	if strings.HasSuffix(location, "/create") || strings.HasSuffix(location, "/edit") {
		t.Errorf("Location = %q, want the record's own page", location)
	}
}

func TestTheButtonIsOfferedOnTheForm(t *testing.T) {
	app, _ := makeApp(t)
	if !strings.Contains(body(t, doGet(t, app, "/admin/users/create", nil)), `name="_addanother"`) {
		t.Error("no Save and add another button on the form")
	}
}
