package gorm

import (
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

func filtered(t *testing.T, admin core.ListQuerier, name, raw string) []string {
	t.Helper()
	got, _ := list(t, admin, core.ListRequest{Filters: map[string]string{name: raw}})
	return got
}

func same(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestBooleanFilter(t *testing.T) {
	db := openDB(t)
	yes, no := true, false
	mustCreate(t, db, &Author{Name: "on", Active: &yes}, &Author{Name: "off", Active: &no}, &Author{Name: "unset"})
	admin := authorAdmin(t, db)
	if got := filtered(t, admin, "Active", "TRUE"); !same(got, "on") {
		t.Errorf("true: %v", got)
	}
	if got := filtered(t, admin, "Active", "false"); !same(got, "off", "unset") {
		t.Errorf("false: %v", got)
	}
}

func TestChoiceFilterConvertsOrMatchesNothing(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &Author{Name: "f", Kind: "fiction"}, &Author{Name: "s", Kind: "science"})
	if got := filtered(t, authorAdmin(t, db), "Kind", "science"); !same(got, "s") {
		t.Errorf("%v", got)
	}
	mustCreate(t, db, &Book{Title: "short", Pages: 100}, &Book{Title: "long", Pages: 200})
	books := bookAdmin(t, db)
	if got := filtered(t, books, "Pages", "200"); !same(got, "long") {
		t.Errorf("200: %v", got)
	}
	if got := filtered(t, books, "Pages", "abc"); len(got) != 0 {
		t.Errorf("abc: %v", got)
	}
}

func TestEmptyFilter(t *testing.T) {
	db := openDB(t)
	blank, nick := "", "N"
	withBook := &Author{Name: "writer"}
	mustCreate(t, db, &Author{Name: "nil"}, &Author{Name: "blank", Nickname: &blank}, &Author{Name: "set", Nickname: &nick}, withBook)
	mustCreate(t, db, &Book{Title: "b", Pages: 1, AuthorID: &withBook.ID})
	admin := authorAdmin(t, db)
	if got := filtered(t, admin, "Nickname", core.EmptyFilterEmpty); !same(got, "nil", "blank", "writer") {
		t.Errorf("empty string column: %v", got)
	}
	if got := filtered(t, admin, "Nickname", core.EmptyFilterNotEmpty); !same(got, "set") {
		t.Errorf("not empty string column: %v", got)
	}
	if got := filtered(t, admin, "Books", core.EmptyFilterNotEmpty); !same(got, "writer") {
		t.Errorf("has-many not empty: %v", got)
	}
	if got := filtered(t, admin, "Books", core.EmptyFilterEmpty); len(got) != 3 {
		t.Errorf("has-many empty: %v", got)
	}
	if got := filtered(t, admin, "Nickname", "bogus"); len(got) != 4 {
		t.Errorf("unknown value narrows: %v", got)
	}
}

func TestEmptyFilterOnBelongsToAndManyToMany(t *testing.T) {
	db := openDB(t)
	author := &Author{Name: "a", Tags: []Tag{{Label: "x"}}}
	mustCreate(t, db, author, &Author{Name: "untagged"})
	mustCreate(t, db, &Book{Title: "owned", Pages: 1, AuthorID: &author.ID}, &Book{Title: "orphan", Pages: 1})
	books, err := New[Book](db, core.BaseModelAdmin{ModelName: "Book", DisplayFields: []string{"Title"},
		DeclaredFilters: []core.Filter{core.NewEmptyFilter("Author")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := filtered(t, books, "Author", core.EmptyFilterEmpty); !same(got, "orphan") {
		t.Errorf("belongs-to: %v", got)
	}
	authors, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", DisplayFields: []string{"Name"},
		DeclaredFilters: []core.Filter{core.NewEmptyFilter("Tags")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := filtered(t, authors, "Tags", core.EmptyFilterEmpty); !same(got, "untagged") {
		t.Errorf("m2m empty: %v", got)
	}
	if got := filtered(t, authors, "Tags", core.EmptyFilterNotEmpty); !same(got, "a") {
		t.Errorf("m2m not empty: %v", got)
	}
}

func TestDateFilterToday(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	old := today.AddDate(0, 0, -40)
	mustCreate(t, db, &Author{Name: "today", Born: &today}, &Author{Name: "old", Born: &old}, &Author{Name: "never"})
	admin := authorAdmin(t, db)
	if got := filtered(t, admin, "Born", "today"); !same(got, "today") {
		t.Errorf("today: %v", got)
	}
	if got := filtered(t, admin, "Born", "nonsense"); len(got) != 3 {
		t.Errorf("unknown preset narrows: %v", got)
	}
}

func TestRelationFilter(t *testing.T) {
	db := openDB(t)
	tag := Tag{Label: "x"}
	ann := &Author{Name: "Ann", Tags: []Tag{tag}}
	bob := &Author{Name: "Bob"}
	mustCreate(t, db, ann, bob)
	book := &Book{Title: "Ann's", Pages: 1, AuthorID: &ann.ID}
	mustCreate(t, db, book, &Book{Title: "Bob's", Pages: 1, AuthorID: &bob.ID})
	books := bookAdmin(t, db)
	if got := filtered(t, books, "Author", "1"); !same(got, "Ann's") {
		t.Errorf("belongs-to: %v", got)
	}
	if got := filtered(t, books, "Author", "abc"); len(got) != 0 {
		t.Errorf("bad key: %v", got)
	}
	if got := filtered(t, authorAdmin(t, db), "Tags", "1"); !same(got, "Ann") {
		t.Errorf("m2m: %v", got)
	}
	byBook, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", DisplayFields: []string{"Name"},
		DeclaredFilters: []core.Filter{core.NewRelationFilter("Books")}})
	if err != nil {
		t.Fatal(err)
	}
	if got := filtered(t, byBook, "Books", "2"); !same(got, "Bob") {
		t.Errorf("has-many: %v", got)
	}
}
