package fiber

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

// queryingUserAdmin implements core.ListQuerier. It records what it was
// asked for and returns a fixed page, so the tests can assert both that
// the framework delegated and that it left the result alone.
//
// GetQueryset deliberately returns a sentinel row that must never reach
// a page: if the framework fell back to the in-memory path, that row
// would show up and the assertions would fail loudly rather than
// silently passing for the wrong reason.
type queryingUserAdmin struct {
	testUserAdmin
	calls         []core.ListRequest
	rows          []any
	total         int
	querysetCalls int
}

func newQueryingUserAdmin() *queryingUserAdmin {
	a := &queryingUserAdmin{testUserAdmin: *newTestUserAdmin()}
	a.rows = []any{
		&testUser{ID: 1, Email: "from-the-querier@example.com", IsActive: true},
	}
	a.total = 137
	return a
}

func (a *queryingUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	a.querysetCalls++
	return []any{&testUser{ID: 99, Email: "IN-MEMORY-FALLBACK@example.com"}}, nil
}

func (a *queryingUserAdmin) ListPage(ctx context.Context, req core.ListRequest) ([]any, int, error) {
	a.calls = append(a.calls, req)
	return a.rows, a.total, nil
}

func TestListQuerierIsUsedInsteadOfTheInMemoryPath(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	page := body(t, doGet(t, app, "/admin/users?search=anything&page=3&page_size=10", nil))

	if len(ma.calls) != 1 {
		t.Fatalf("expected exactly one ListPage call, got %d", len(ma.calls))
	}
	if ma.querysetCalls != 0 {
		t.Error("GetQueryset must not be called when ListPage handles the query")
	}
	if !strings.Contains(page, "from-the-querier@example.com") {
		t.Error("the querier's rows should be what renders")
	}
	if strings.Contains(page, "IN-MEMORY-FALLBACK") {
		t.Error("the framework fell back to filtering GetQueryset in memory")
	}
}

func TestListQuerierReceivesTheWholeRequest(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	doGet(t, app, "/admin/users?search=jane&sort=-Email&page=3&page_size=10&filter[IsActive]=true", nil)

	got := ma.calls[0]
	if got.Search != "jane" || got.Ordering != "-Email" {
		t.Errorf("search/ordering not passed through: %+v", got)
	}
	if got.Filters["IsActive"] != "true" {
		t.Errorf("filters not passed through: %+v", got.Filters)
	}
	offset, limit := got.Window()
	if offset != 20 || limit != 10 {
		t.Errorf("window = (%d,%d), want (20,10)", offset, limit)
	}
}

// The count the querier reports is what pagination believes -- it is
// the whole point of returning it separately from the rows.
func TestListQuerierTotalDrivesPagination(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	page := body(t, doGet(t, app, "/admin/users?page_size=10", nil))
	if !strings.Contains(page, strconv.Itoa(ma.total)) {
		t.Errorf("expected the querier's total (%d) to reach the page", ma.total)
	}
}

// Export asks for every matching row, not the page the user was on.
func TestExportAsksTheQuerierForEverything(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	doGet(t, app, "/admin/users/export/csv?page=4&page_size=10&search=jane", nil)

	got := ma.calls[0]
	if !got.Unlimited {
		t.Error("an export must ask for every matching row")
	}
	if offset, limit := got.Window(); offset != 0 || limit != 0 {
		t.Errorf("window = (%d,%d), want (0,0) -- no page window on an export", offset, limit)
	}
	if got.Search != "jane" {
		t.Error("the export must keep the filters the user was looking at")
	}
}

// The autocomplete caps its suggestions through the window, so a
// querier applies the limit in its own query.
func TestLookupAsksTheQuerierForACappedPage(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	doGet(t, app, "/admin/users/lookup?q=jan", nil)

	got := ma.calls[0]
	if got.Search != "jan" {
		t.Errorf("search not passed through: %q", got.Search)
	}
	if _, limit := got.Window(); limit != lookupLimit {
		t.Errorf("limit = %d, want %d", limit, lookupLimit)
	}
}

// And the fallback still works: an admin with no ListPage keeps the
// in-memory pipeline, windowed the same way.
//
// Both requests pin an explicit sort. The fixture stores rows in a map
// and the framework has no default ordering, so without one each
// request windows over a freshly randomised order and the two pages
// legitimately overlap -- a property of the fixture, not a paging bug.
// (That missing default ordering is a real gap; see the roadmap.)
func TestAdminWithoutListPageStillPaginatesInMemory(t *testing.T) {
	userAdmin := newTestUserAdmin()
	for i := 0; i < 25; i++ {
		userAdmin.createUser("user"+strconv.Itoa(i)+"@example.com", true)
	}
	app := newTestApp(t, core.New(core.WithModelAdmins(userAdmin)))

	emailsOn := func(path string) map[string]bool {
		found := map[string]bool{}
		for _, m := range emailPattern.FindAllStringSubmatch(body(t, doGet(t, app, path, nil)), -1) {
			found[m[0]] = true
		}
		return found
	}
	first := emailsOn("/admin/users?sort=Email&page=1&page_size=10")
	second := emailsOn("/admin/users?sort=Email&page=2&page_size=10")

	if len(first) != 10 || len(second) != 10 {
		t.Fatalf("expected 10 rows per page, got %d and %d", len(first), len(second))
	}
	for email := range second {
		if first[email] {
			t.Errorf("%s appears on both pages -- the window is not advancing", email)
		}
	}
}

var emailPattern = regexp.MustCompile(`user\d+@example\.com`)
