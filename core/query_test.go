package core

import (
	"context"
	"testing"
)

func mustCreateUser(t *testing.T, admin inMemoryUserAdmin, email string, isActive bool) *User {
	t.Helper()
	obj, err := admin.Create(context.Background(), map[string]any{"Email": email, "IsActive": isActive})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return obj.(*User)
}

func usersQueryset(t *testing.T, admin inMemoryUserAdmin) []any {
	t.Helper()
	qs, err := admin.GetQueryset(context.Background())
	if err != nil {
		t.Fatalf("get queryset: %v", err)
	}
	users := qs.([]*User)
	out := make([]any, len(users))
	for i, u := range users {
		out[i] = u
	}
	return out
}

func TestApplySearchCaseInsensitive(t *testing.T) {
	admin := newInMemoryUserAdmin()
	john := mustCreateUser(t, admin, "John@Example.com", true)
	mustCreateUser(t, admin, "mary@example.com", true)

	result := ApplySearch(admin, usersQueryset(t, admin), "john")
	if len(result) != 1 || result[0].(*User) != john {
		t.Fatalf("got %v", result)
	}
}

func TestApplySearchEmptyIsNoop(t *testing.T) {
	admin := newInMemoryUserAdmin()
	mustCreateUser(t, admin, "john@example.com", true)

	objects := usersQueryset(t, admin)
	result := ApplySearch(admin, objects, "")
	if len(result) != len(objects) {
		t.Fatalf("got %v", result)
	}
}

func TestApplyFiltersByName(t *testing.T) {
	admin := newInMemoryUserAdmin()
	admin.DeclaredFilters = []Filter{NewBooleanFilter("IsActive")}
	active := mustCreateUser(t, admin, "a@example.com", true)
	mustCreateUser(t, admin, "b@example.com", false)

	result := ApplyFilters(admin, usersQueryset(t, admin), map[string]string{"IsActive": "true"})
	if len(result) != 1 || result[0].(*User) != active {
		t.Fatalf("got %v", result)
	}
}

func TestApplyOrderingAscendingAndDescending(t *testing.T) {
	admin := newInMemoryUserAdmin()
	b := mustCreateUser(t, admin, "b@example.com", true)
	a := mustCreateUser(t, admin, "a@example.com", true)

	asc := ApplyOrdering(admin, usersQueryset(t, admin), "Email")
	if asc[0].(*User) != a || asc[1].(*User) != b {
		t.Fatalf("got %v", asc)
	}

	desc := ApplyOrdering(admin, usersQueryset(t, admin), "-Email")
	if desc[0].(*User) != b || desc[1].(*User) != a {
		t.Fatalf("got %v", desc)
	}
}

func TestApplyOrderingUnknownFieldIsNoop(t *testing.T) {
	admin := newInMemoryUserAdmin()
	mustCreateUser(t, admin, "a@example.com", true)

	objects := usersQueryset(t, admin)
	result := ApplyOrdering(admin, objects, "Nope")
	if len(result) != len(objects) {
		t.Fatalf("got %v", result)
	}
}

func TestExecuteListQueryComposesSearchFilterOrdering(t *testing.T) {
	admin := newInMemoryUserAdmin()
	admin.DeclaredFilters = []Filter{NewBooleanFilter("IsActive")}
	mustCreateUser(t, admin, "zzz@example.com", false)
	matchB := mustCreateUser(t, admin, "match-b@example.com", true)
	matchA := mustCreateUser(t, admin, "match-a@example.com", true)

	result := ExecuteListQuery(admin, usersQueryset(t, admin), ListRequest{
		Search:   "match",
		Filters:  map[string]string{"IsActive": "true"},
		Ordering: "Email",
	})
	if len(result) != 2 || result[0].(*User) != matchA || result[1].(*User) != matchB {
		t.Fatalf("got %v", result)
	}
}

// -- the ListQuerier capability ------------------------------------------

func TestListWindowDerivesOffsetAndLimitFromThePage(t *testing.T) {
	cases := []struct {
		req           ListRequest
		offset, limit int
	}{
		{ListRequest{Page: 1, PageSize: 25}, 0, 25},
		{ListRequest{Page: 3, PageSize: 10}, 20, 10},
		// Page/PageSize unset is "the first page of the default size",
		// matching what the handlers already do with a bare request.
		{ListRequest{}, 0, DefaultPageSize},
		// PageSize 0 with an explicit Unlimited is how export asks for
		// every matching row rather than a page of them.
		{ListRequest{Unlimited: true}, 0, 0},
	}
	for _, c := range cases {
		offset, limit := c.req.Window()
		if offset != c.offset || limit != c.limit {
			t.Errorf("%+v: got (%d,%d), want (%d,%d)", c.req, offset, limit, c.offset, c.limit)
		}
	}
}

func TestUnlimitedWindowIgnoresThePageNumber(t *testing.T) {
	// "Every matching row" cannot also be "starting from row 40" -- an
	// export of a filtered set is the whole set, whichever page the user
	// happened to be looking at when they clicked Export.
	offset, limit := ListRequest{Page: 5, PageSize: 10, Unlimited: true}.Window()
	if offset != 0 || limit != 0 {
		t.Errorf("got (%d,%d), want (0,0)", offset, limit)
	}
}
