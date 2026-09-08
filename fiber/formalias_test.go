package fiber

import (
	"net/url"
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

var _ = core.DefaultPageSize
