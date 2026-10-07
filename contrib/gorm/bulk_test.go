package gorm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestBulkDeleteStopsAtARefusedRecord(t *testing.T) {
	db := openDB(t)
	free, held, other := &Author{Name: "free"}, &Author{Name: "held"}, &Author{Name: "other"}
	mustCreate(t, db, free, held, other, &Book{Title: "b", Pages: 1, AuthorID: &held.ID})
	admin := authorAdmin(t, db)
	action, _ := core.GetAction(admin, core.DeleteSelectedName)
	_, err := action.Run(context.Background(), admin, []any{free, held, other}, nil, nil)
	var actionErr *core.ActionError
	if !errors.As(err, &actionErr) || len(actionErr.Done) != 1 || !strings.HasSuffix(actionErr.Message, "Other records still refer to this one.") {
		t.Fatalf("%T %v", err, err)
	}
	if obj, _ := admin.GetObject(context.Background(), "3"); obj == nil {
		t.Fatal("the record after the failure was deleted")
	}
}

func TestBulkEditStopsAtADuplicate(t *testing.T) {
	db := openDB(t)
	a, b := &Author{Name: "a"}, &Author{Name: "b"}
	mustCreate(t, db, a, b)
	admin, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", DisplayFields: []string{"Name"},
		FormFieldNames: []string{"Name"}, BulkEditFieldNames: []string{"Name"}})
	if err != nil {
		t.Fatal(err)
	}
	action, _ := core.GetAction(admin, core.BulkEditName)
	_, err = action.Run(context.Background(), admin, []any{a, b}, map[string]any{"Name": "same"}, nil)
	var actionErr *core.ActionError
	if !errors.As(err, &actionErr) || actionErr.Message != "Updated 1 of 2, then failed: A record with these values already exists." {
		t.Fatalf("%T %v", err, err)
	}
}
