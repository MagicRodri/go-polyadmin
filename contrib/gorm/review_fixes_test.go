package gorm

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	gormdb "gorm.io/gorm"
)

func TestValuesSetByHooksAreWritten(t *testing.T) {
	db := openDB(t)
	admin, err := New[Gadget](db, core.BaseModelAdmin{ModelName: "Gadget", DisplayFields: []string{"Name"}, FormFieldNames: []string{"Name"}})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := admin.Create(context.Background(), map[string]any{"Name": "Box"})
	if err != nil || obj == nil {
		t.Fatalf("create: %v %v", obj, err)
	}
	var stored Gadget
	db.First(&stored, "id = ?", "g-Box")
	if stored.Slug != "box" {
		t.Fatalf("create lost the hook's slug: %+v", stored)
	}
	if _, err := admin.Update(context.Background(), obj, map[string]any{"Name": "Crate"}); err != nil {
		t.Fatal(err)
	}
	db.First(&stored, "id = ?", "g-Box")
	if stored.Slug != "crate" {
		t.Fatalf("update lost the hook's slug: %+v", stored)
	}
}

func TestANilRelatedRecordClearsTheForeignKey(t *testing.T) {
	db := openDB(t)
	ann := &Author{Name: "Ann"}
	mustCreate(t, db, ann)
	book := &Book{Title: "x", Pages: 1, AuthorID: &ann.ID}
	mustCreate(t, db, book)
	obj, err := bookAdmin(t, db).Update(context.Background(), book, map[string]any{"Author": (*Author)(nil)})
	if err != nil || obj.(*Book).AuthorID != nil {
		t.Fatalf("%+v %v", obj, err)
	}
	got, err := convert((*string)(nil), reflect.TypeFor[*string]())
	if err != nil || !got.IsNil() {
		t.Fatalf("convert(nil *string) = %v, %v", got, err)
	}
}

func TestSelfReferentialHasManyFilter(t *testing.T) {
	db := openDB(t)
	boss := &Employee{Name: "boss"}
	mustCreate(t, db, boss)
	mustCreate(t, db, &Employee{Name: "report", ManagerID: &boss.ID})
	admin, err := New[Employee](db, core.BaseModelAdmin{ModelName: "Employee", DisplayFields: []string{"Name"},
		DeclaredFilters: []core.Filter{core.NewEmptyFilter("Reports")}})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := admin.ListPage(context.Background(), core.ListRequest{Filters: map[string]string{"Reports": core.EmptyFilterNotEmpty}})
	if err != nil || len(rows) != 1 || rows[0].(*Employee).Name != "boss" {
		t.Fatalf("not empty: %+v %v", rows, err)
	}
	rows, _, _ = admin.ListPage(context.Background(), core.ListRequest{Filters: map[string]string{"Reports": core.EmptyFilterEmpty}})
	if len(rows) != 1 || rows[0].(*Employee).Name != "report" {
		t.Fatalf("empty: %+v", rows)
	}
}

func TestARefusedDeleteKeepsItsManyToManyLinks(t *testing.T) {
	db := openDBWith(t, &gormdb.Config{SkipDefaultTransaction: true})
	ann := &Author{Name: "Ann", Tags: []Tag{{Label: "x"}}}
	mustCreate(t, db, ann, &Book{Title: "b", Pages: 1, AuthorID: &ann.ID})
	if err := authorAdmin(t, db).Delete(context.Background(), ann); err == nil {
		t.Fatal("the delete was not refused")
	}
	var links int64
	db.Table("author_tags").Count(&links)
	if links != 1 {
		t.Fatalf("the refused delete removed %d link(s)", 1-links)
	}
}

func TestDateFilterMatchesDateOnlyText(t *testing.T) {
	db := openDB(t)
	mustCreate(t, db, &Author{Name: "plain"})
	db.Exec("UPDATE authors SET born = ? WHERE name = ?", time.Now().Format("2006-01-02"), "plain")
	if got := filtered(t, authorAdmin(t, db), "Born", core.DateFilterToday); !same(got, "plain") {
		t.Fatalf("%v", got)
	}
}
