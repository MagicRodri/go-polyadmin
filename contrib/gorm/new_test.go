package gorm

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

type customFilter struct{ core.BooleanFilter }

func TestNewRejectsMisconfiguration(t *testing.T) {
	db := openDB(t)
	cases := map[string]func() error{
		"not a struct": func() error { _, err := New[int](db, core.BaseModelAdmin{ModelName: "Int"}); return err },
		"composite primary key": func() error {
			_, err := New[CompositeKey](db, core.BaseModelAdmin{ModelName: "CompositeKey"})
			return err
		},
		"filter with no SQL translation": func() error {
			_, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author",
				DeclaredFilters: []core.Filter{customFilter{core.NewBooleanFilter("Active")}}})
			return err
		},
		"filter naming nothing": func() error {
			_, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author",
				DeclaredFilters: []core.Filter{core.NewBooleanFilter("Missing")}})
			return err
		},
		"default filter naming no column": func() error {
			_, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author"}, WithDefaultFilters(map[string]any{"Books": 1}))
			return err
		},
		"has-many form field": func() error {
			_, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", FormFieldNames: []string{"Books"}})
			return err
		},
		"many-to-many form field": func() error {
			_, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", FormFieldNames: []string{"Tags"}})
			return err
		},
	}
	for name, build := range cases {
		if err := build(); err == nil {
			t.Errorf("%s: New accepted it", name)
		}
	}
}

func TestNewDerivesFieldsFromTheSchema(t *testing.T) {
	db := openDB(t)
	authors, books := authorAdmin(t, db), bookAdmin(t, db)
	want := map[string]struct {
		typ      core.FieldType
		required bool
	}{
		"Name":     {core.FieldTypeString, true},
		"Active":   {core.FieldTypeBoolean, false},
		"Born":     {core.FieldTypeDate, false},
		"Created":  {core.FieldTypeDateTime, false},
		"Kind":     {core.FieldTypeString, false},
		"Nickname": {core.FieldTypeString, false},
		"ID":       {core.FieldTypeInteger, false},
	}
	for name, w := range want {
		f, ok := authors.Field(name)
		if !ok || f.Type != w.typ || f.Required != w.required {
			t.Errorf("%s: got %+v ok=%v, want type %s required %v", name, f, ok, w.typ, w.required)
		}
	}
	if f, _ := books.Field("Pages"); f.Type != core.FieldTypeInteger || !f.Required {
		t.Errorf("Pages: %+v", f)
	}
	if f, _ := books.Field("Author"); f.Type != core.FieldTypeForeignKey {
		t.Errorf("a declared field was replaced: %+v", f)
	}
	if errs := authors.Validate(t.Context(), map[string]any{}); len(errs["Name"]) == 0 {
		t.Errorf("Validate does not see the derived required Name: %v", errs)
	}
}

func TestGetPKReadsTheSchemaPrimaryKey(t *testing.T) {
	admin := authorAdmin(t, openDB(t))
	if got := admin.GetPK(&Author{ID: 7}); got != uint(7) {
		t.Fatalf("got %#v", got)
	}
	if got := admin.GetPK((*Author)(nil)); got != nil {
		t.Fatalf("got %#v", got)
	}
}

func TestDefaultFiltersAreExposed(t *testing.T) {
	admin := authorAdmin(t, openDB(t), WithDefaultFilters(map[string]any{"Active": true}))
	var filterer core.DefaultFilterer = admin
	if filterer.DefaultFilters()["Active"] != true {
		t.Fatal(filterer.DefaultFilters())
	}
	if !strings.Contains(admin.Slug(), "author") {
		t.Fatal(admin.Slug())
	}
}
