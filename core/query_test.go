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
