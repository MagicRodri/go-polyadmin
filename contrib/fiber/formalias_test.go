package fiber

import (
	"context"
	"net/url"
	"strconv"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestStoredFormValuesSurviveTheNextRequest(t *testing.T) {
	app, userAdmin := makeApp(t)

	first := "aaa@example.com"
	second := "bbb@example.com"
	doPostForm(t, app, "/admin/users/create", url.Values{"Email": {first}}, nil)
	doPostForm(t, app, "/admin/users/create", url.Values{"Email": {second}}, nil)

	seen := map[string]int{}
	for _, u := range userAdmin.store {
		seen[u.Email]++
	}
	if seen[first] != 1 {
		t.Errorf("the first record's email is gone (found %d copies of %q, %d of %q) -- "+
			"a stored form value aliased the request buffer", seen[first], first, seen[second], second)
	}
	if seen[second] != 1 {
		t.Errorf("expected exactly one %q, got %d", second, seen[second])
	}
}

func TestStoredPathParamsSurviveTheNextRequest(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.store[1] = &testUser{ID: 1, Email: "a@example.com"}
	userAdmin.store[2] = &testUser{ID: 2, Email: "b@example.com"}

	// Two detail reads back to back; the second must not disturb what
	// the first handed the ModelAdmin.
	if resp := doGet(t, app, "/admin/users/1", nil); resp.StatusCode != 200 {
		t.Fatalf("first read got %d", resp.StatusCode)
	}
	if resp := doGet(t, app, "/admin/users/2", nil); resp.StatusCode != 200 {
		t.Fatalf("second read got %d", resp.StatusCode)
	}
	if userAdmin.store[1].Email != "a@example.com" {
		t.Errorf("record 1's email became %q", userAdmin.store[1].Email)
	}
}

// The list query reaches a ListQuerier -- the database-backed path -- so
// one that keeps the request past its handler (a query cache, an audit
// row) must not see it rewritten by the next request. Same-length values
// on purpose: they occupy the same bytes of the reused buffer.
func TestListRequestSurvivesTheNextRequest(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	doGet(t, app, "/admin/users?search=aaaa&sort=-Email", nil)
	doGet(t, app, "/admin/users?search=bbbb&sort=-ZZZZZ", nil)

	if got := ma.calls[0]; got.Search != "aaaa" || got.Ordering != "-Email" {
		t.Errorf("the first ListRequest became search=%q sort=%q -- it aliased the request buffer",
			got.Search, got.Ordering)
	}
}

func TestSelectAllRequestSurvivesTheNextRequest(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	post := func(search, sort string) {
		form := url.Values{selectAllField: {"1"}, "search": {search}, "sort": {sort}}
		doPostForm(t, app, "/admin/users/actions/"+core.DeleteSelectedName, form, nil)
	}
	post("aaaa", "-Email")
	post("bbbb", "-ZZZZZ")

	if got := ma.calls[0]; got.Search != "aaaa" || got.Ordering != "-Email" {
		t.Errorf("the first select-all ListRequest became search=%q sort=%q -- it aliased the request buffer",
			got.Search, got.Ordering)
	}
}

func TestLookupSearchSurvivesTheNextRequest(t *testing.T) {
	ma := newQueryingUserAdmin()
	app := newTestApp(t, core.New(core.WithModelAdmins(ma)))

	doGet(t, app, "/admin/users/lookup?q=aaaa", nil)
	doGet(t, app, "/admin/users/lookup?q=bbbb", nil)

	if got := ma.calls[0].Search; got != "aaaa" {
		t.Errorf("the first lookup's search became %q -- it aliased the request buffer", got)
	}
}

// recordingLoginBackend keeps every identifier it is shown, the way a
// failed-attempt log or a per-account rate limiter would.
type recordingLoginBackend struct {
	*fakeLoginBackend
	identifiers []string
}

func (b *recordingLoginBackend) VerifyCredentials(request any, identifier, password string) *core.Principal {
	b.identifiers = append(b.identifiers, identifier)
	return b.fakeLoginBackend.VerifyCredentials(request, identifier, password)
}

func TestLoginIdentifierSurvivesTheNextRequest(t *testing.T) {
	backend := &recordingLoginBackend{fakeLoginBackend: &fakeLoginBackend{password: "correct horse"}}
	app := newTestApp(t, core.New(
		core.WithModelAdmins(newTestUserAdmin()),
		core.WithAuthenticator(backend),
		core.WithLoginBackend(backend),
	))

	for _, who := range []string{"aaa@example.com", "bbb@example.com"} {
		doPostForm(t, app, "/admin/login", url.Values{"identifier": {who}, "password": {"wrong"}}, nil)
	}

	if len(backend.identifiers) == 0 || backend.identifiers[0] != "aaa@example.com" {
		t.Errorf("the first identifier became %v -- it aliased the request buffer", backend.identifiers)
	}
}

// recordingInlineUserAdmin keeps every pk it is asked for, the way a
// cache keyed on it would.
type recordingInlineUserAdmin struct {
	*inlineTestUserAdmin
	asked []any
}

func (a *recordingInlineUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	a.asked = append(a.asked, pk)
	return a.inlineTestUserAdmin.GetObject(ctx, pk)
}

func TestInlineChildPKSurvivesTheNextRequest(t *testing.T) {
	orgAdmin := newInlineTestOrgAdmin(core.InlineLayoutTabular)
	userAdmin := &recordingInlineUserAdmin{inlineTestUserAdmin: newInlineTestUserAdmin(orgAdmin)}
	app := newTestApp(t, core.New(core.WithModelAdmins(orgAdmin, userAdmin)))
	org, users := seedOrgWithUsers(orgAdmin, userAdmin.inlineTestUserAdmin, "a@example.com", "b@example.com")

	base := "/admin/organizations/" + strconv.Itoa(org.ID) + "/inlines/users/"
	first, second := strconv.Itoa(users[0].ID), strconv.Itoa(users[1].ID)
	doPostForm(t, app, base+first, url.Values{"Email": {"a@example.com"}}, nil)
	doPostForm(t, app, base+second, url.Values{"Email": {"b@example.com"}}, nil)

	if len(userAdmin.asked) == 0 || userAdmin.asked[0] != first {
		t.Errorf("the first inline childPK became %v, want %q -- it aliased the request buffer",
			userAdmin.asked, first)
	}
}
