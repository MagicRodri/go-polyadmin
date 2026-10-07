package gorm

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func recordErr(t *testing.T, err error) map[string][]string {
	t.Helper()
	var re *core.RecordFormError
	if !errors.As(err, &re) {
		t.Fatalf("got %T %v, want a RecordFormError", err, err)
	}
	return re.Errors
}

func TestCreateConvertsAndReloads(t *testing.T) {
	db := openDB(t)
	obj, err := authorAdmin(t, db).Create(context.Background(), map[string]any{
		"Name": "Ann", "Active": true, "Kind": "science", "Born": "2020-01-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	a := obj.(*Author)
	if a.ID == 0 || a.Kind != "science" || a.Active == nil || !*a.Active || a.Born == nil || a.Born.Year() != 2020 {
		t.Fatalf("%+v", a)
	}
}

func TestCreateWritesFalseOverAColumnDefault(t *testing.T) {
	db := openDB(t)
	obj, err := bookAdmin(t, db).Create(context.Background(), map[string]any{"Title": "x", "Pages": 1, "Published": false})
	if err != nil {
		t.Fatal(err)
	}
	var stored Book
	db.First(&stored, obj.(*Book).ID)
	if stored.Published {
		t.Fatal("the column default replaced an explicit false")
	}
}

func TestWritesFireHooks(t *testing.T) {
	db := openDB(t)
	hookLog = nil
	admin := bookAdmin(t, db)
	obj, err := admin.Create(context.Background(), map[string]any{"Title": "x", "Pages": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Update(context.Background(), obj, map[string]any{"Title": "y"}); err != nil {
		t.Fatal(err)
	}
	if err := admin.Delete(context.Background(), obj); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"after_create", "after_update", "after_delete"} {
		if !slices.Contains(hookLog, want) {
			t.Errorf("%s did not fire: %v", want, hookLog)
		}
	}
}

func TestUpdateWritesZeroValuesAndForeignKeys(t *testing.T) {
	db := openDB(t)
	ann, bob := &Author{Name: "Ann"}, &Author{Name: "Bob"}
	mustCreate(t, db, ann, bob)
	book := &Book{Title: "x", Pages: 100, AuthorID: &ann.ID}
	mustCreate(t, db, book)
	admin := bookAdmin(t, db)
	obj, err := admin.Update(context.Background(), book, map[string]any{"Pages": 0, "Author": "2"})
	if err != nil {
		t.Fatal(err)
	}
	got := obj.(*Book)
	if got.Pages != 0 || got.AuthorID == nil || *got.AuthorID != bob.ID || got.Author == nil || got.Author.Name != "Bob" {
		t.Fatalf("%+v", got)
	}
	obj, err = admin.Update(context.Background(), got, map[string]any{"Author": nil})
	if err != nil || obj.(*Book).AuthorID != nil {
		t.Fatalf("clearing the FK: %+v %v", obj, err)
	}
}

func TestAssignAcceptsTheRelatedRecord(t *testing.T) {
	db := openDB(t)
	ann := &Author{Name: "Ann"}
	mustCreate(t, db, ann)
	obj, err := bookAdmin(t, db).Create(context.Background(), map[string]any{"Title": "x", "Pages": 1, "Author": ann})
	if err != nil || obj.(*Book).AuthorID == nil || *obj.(*Book).AuthorID != ann.ID {
		t.Fatalf("%+v %v", obj, err)
	}
}

func TestConversionErrorsAreFieldErrors(t *testing.T) {
	db := openDB(t)
	_, err := bookAdmin(t, db).Create(context.Background(), map[string]any{"Title": "x", "Pages": "abc"})
	if got := recordErr(t, err); len(got["Pages"]) != 1 || got["Pages"][0] != "Enter a valid value." {
		t.Fatalf("%v", got)
	}
	var count int64
	db.Model(&Book{}).Count(&count)
	if count != 0 {
		t.Fatal("a row was written")
	}
}

func TestConstraintViolationsBecomeRecordFormErrors(t *testing.T) {
	db := openDB(t)
	admin := authorAdmin(t, db)
	ann := &Author{Name: "Ann"}
	mustCreate(t, db, ann, &Book{Title: "b", Pages: 1, AuthorID: &ann.ID})
	_, err := admin.Create(context.Background(), map[string]any{"Name": "Ann"})
	if got := recordErr(t, err)[""]; len(got) != 1 || got[0] != "A record with these values already exists." {
		t.Errorf("duplicate: %v", got)
	}
	_, err = bookAdmin(t, db).Create(context.Background(), map[string]any{"Title": "x", "Pages": 1, "Author": "999"})
	if got := recordErr(t, err)[""]; len(got) != 1 || got[0] != "The related record does not exist." {
		t.Errorf("fk on save: %v", got)
	}
	err = admin.Delete(context.Background(), ann)
	if got := recordErr(t, err)[""]; len(got) != 1 || got[0] != "Other records still refer to this one." {
		t.Errorf("fk on delete: %v", got)
	}
	if obj, _ := admin.GetObject(context.Background(), "1"); obj == nil {
		t.Error("the refused delete removed the row")
	}
}

func TestUpdatingAGoneRecord(t *testing.T) {
	db := openDB(t)
	admin := authorAdmin(t, db)
	ann := &Author{Name: "Ann"}
	mustCreate(t, db, ann)
	db.Delete(&Author{}, ann.ID)
	_, err := admin.Update(context.Background(), ann, map[string]any{"Name": "x"})
	if got := recordErr(t, err)[""]; len(got) != 1 || got[0] != "The record no longer exists." {
		t.Fatalf("%v", got)
	}
	if err := admin.Delete(context.Background(), ann); err != nil {
		t.Fatalf("deleting a gone record: %v", err)
	}
}

func TestDeleteRemovesManyToManyLinks(t *testing.T) {
	db := openDB(t)
	ann := &Author{Name: "Ann", Tags: []Tag{{Label: "x"}}}
	mustCreate(t, db, ann)
	if err := authorAdmin(t, db).Delete(context.Background(), ann); err != nil {
		t.Fatal(err)
	}
	var links, tags int64
	db.Table("author_tags").Count(&links)
	db.Model(&Tag{}).Count(&tags)
	if links != 0 || tags != 1 {
		t.Fatalf("links %d tags %d", links, tags)
	}
}
