package gorm

import (
	"context"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func names(rows []any) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		switch v := r.(type) {
		case *Author:
			out[i] = v.Name
		case *Book:
			out[i] = v.Title
		}
	}
	return out
}

func list(t *testing.T, admin core.ListQuerier, req core.ListRequest) ([]string, int) {
	t.Helper()
	rows, total, err := admin.ListPage(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return names(rows), total
}

func TestSearchMatchesPercentAndUnderscoreLiterallyAndIgnoresCase(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &Author{Name: "100% pure"}, &Author{Name: "100 pure"}, &Author{Name: "a_b"}, &Author{Name: "axb"})
	admin := authorAdmin(t, db)
	if got, _ := list(t, admin, core.ListRequest{Search: "100%"}); len(got) != 1 || got[0] != "100% pure" {
		t.Errorf("100%%: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Search: "a_b"}); len(got) != 1 || got[0] != "a_b" {
		t.Errorf("a_b: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Search: "PURE"}); len(got) != 2 {
		t.Errorf("PURE: %v", got)
	}
}

func TestSearchCoversNullableColumns(t *testing.T) {
	db := openDB(t)
	nick := "Shadow"
	mustCreate(t, db, &Author{Name: "Ann", Nickname: &nick}, &Author{Name: "Bob"})
	if got, _ := list(t, authorAdmin(t, db), core.ListRequest{Search: "shad"}); len(got) != 1 || got[0] != "Ann" {
		t.Errorf("%v", got)
	}
}

func TestOrderingByAColumnThenThePrimaryKey(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &Author{Name: "c", Kind: "fiction"}, &Author{Name: "a", Kind: "science"}, &Author{Name: "b", Kind: "fiction"})
	admin := authorAdmin(t, db)
	if got, _ := list(t, admin, core.ListRequest{Ordering: "-Kind"}); got[0] != "a" || got[1] != "c" || got[2] != "b" {
		t.Errorf("-Kind: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Ordering: "Name"}); got[0] != "a" || got[2] != "c" {
		t.Errorf("Name: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Ordering: "Bogus"}); got[0] != "c" {
		t.Errorf("unknown ordering: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Ordering: "-ID"}); got[0] != "b" {
		t.Errorf("-ID: %v", got)
	}
}

func TestWindowAndTotal(t *testing.T) {
	db := openDB(t)
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		mustCreate(t, db, &Author{Name: n})
	}
	got, total := list(t, authorAdmin(t, db), core.ListRequest{Page: 2, PageSize: 2})
	if total != 5 || len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Errorf("got %v total %d", got, total)
	}
	got, total = list(t, authorAdmin(t, db), core.ListRequest{Unlimited: true})
	if total != 5 || len(got) != 5 {
		t.Errorf("unlimited: %v %d", got, total)
	}
}

func TestDefaultFiltersApplyUntilTheFilterIsChosen(t *testing.T) {
	db := openDB(t)
	yes, no := true, false
	mustCreate(t, db, &Author{Name: "on", Active: &yes}, &Author{Name: "off", Active: &no})
	admin := authorAdmin(t, db, WithDefaultFilters(map[string]any{"Active": true}))
	if got, _ := list(t, admin, core.ListRequest{}); len(got) != 1 || got[0] != "on" {
		t.Errorf("default: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Filters: map[string]string{"Active": ""}}); len(got) != 2 {
		t.Errorf("All: %v", got)
	}
	if got, _ := list(t, admin, core.ListRequest{Filters: map[string]string{"Active": "false"}}); len(got) != 1 || got[0] != "off" {
		t.Errorf("false: %v", got)
	}
}

func TestRelationshipsAreLoaded(t *testing.T) {
	db := openDB(t)
	author := &Author{Name: "Ann"}
	mustCreate(t, db, author, &Book{Title: "One", Pages: 1, AuthorID: &author.ID})
	rows, _, err := bookAdmin(t, db).ListPage(context.Background(), core.ListRequest{})
	if err != nil || len(rows) != 1 || rows[0].(*Book).Author == nil || rows[0].(*Book).Author.Name != "Ann" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
}

func TestGetObjectAndGetQueryset(t *testing.T) {
	db := openDB(t)
	a := &Author{Name: "Ann"}
	mustCreate(t, db, a, &Author{Name: "Bob"})
	admin := authorAdmin(t, db)
	obj, err := admin.GetObject(context.Background(), "1")
	if err != nil || obj.(*Author).Name != "Ann" {
		t.Fatalf("%v %v", obj, err)
	}
	for _, pk := range []any{"", "abc", "99999999999999999999", "12345", nil} {
		if obj, err := admin.GetObject(context.Background(), pk); obj != nil || err != nil {
			t.Errorf("GetObject(%#v) = %v, %v", pk, obj, err)
		}
	}
	all, err := admin.GetQueryset(context.Background())
	if err != nil || len(all.([]any)) != 2 {
		t.Fatalf("%v %v", all, err)
	}
}
