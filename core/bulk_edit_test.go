package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type bulkItem struct {
	ID       int
	Email    string
	IsActive bool
}

type bulkAdmin struct {
	BaseModelAdmin
	updates []map[string]any
	failOn  int
}

func newBulkAdmin(names ...string) *bulkAdmin {
	return &bulkAdmin{BaseModelAdmin: BaseModelAdmin{
		ModelName:      "Item",
		FormFieldNames: []string{"Email", "IsActive"},
		DeclaredFields: []Field{
			NewField("Email", FieldTypeEmail, WithRequired()),
			NewField("IsActive", FieldTypeBoolean),
		},
		BulkEditFieldNames: names,
	}}
}

func (a *bulkAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	item := obj.(*bulkItem)
	if item.ID == a.failOn {
		return nil, errors.New("boom")
	}
	a.updates = append(a.updates, data)
	if v, ok := data["IsActive"]; ok {
		item.IsActive = v.(bool)
	}
	return item, nil
}

func TestNoBulkEditWithoutFields(t *testing.T) {
	if _, ok := GetAction(newBulkAdmin(), BulkEditName); ok {
		t.Fatal("bulk_edit offered without fields")
	}
}

func TestBulkEditSitsBeforeDeleteSelected(t *testing.T) {
	names := actionNames(newBulkAdmin("IsActive").Actions())
	if strings.Join(names, ",") != "bulk_edit,delete_selected" {
		t.Fatalf("got %v", names)
	}
}

func TestBulkEditFormUsesTheModelAdminsFields(t *testing.T) {
	a, _ := GetAction(newBulkAdmin("IsActive"), BulkEditName)
	if len(a.Form) != 1 || a.Form[0].Name != "IsActive" || a.Permission != "update" || a.Where != ActionWhereList {
		t.Fatalf("got %+v", a)
	}
}

func TestBulkEditNeedsCanUpdate(t *testing.T) {
	ma := newBulkAdmin("IsActive")
	ma.DisableUpdate = true
	if _, ok := GetAction(ma, BulkEditName); ok {
		t.Fatal("bulk_edit offered without update")
	}
}

func TestBulkEditIsNeverADetailAction(t *testing.T) {
	ma := newBulkAdmin("IsActive")
	ma.DeclaredDetailActions = []string{BulkEditName}
	if len(ActionsForDetail(ma)) != 0 {
		t.Fatal("bulk_edit on detail page")
	}
}

func TestDefaultBulkUpdatePassesOnlyGivenData(t *testing.T) {
	ma := newBulkAdmin("IsActive")
	a, _ := GetAction(ma, BulkEditName)
	items := []any{&bulkItem{ID: 1, Email: "a@x.com", IsActive: true}, &bulkItem{ID: 2, Email: "b@x.com", IsActive: true}}
	res, err := a.Run(context.Background(), ma, items, map[string]any{"IsActive": false}, nil)
	if err != nil || res.Message != "Updated 2 records." {
		t.Fatalf("got %+v %v", res, err)
	}
	if len(ma.updates) != 2 || len(ma.updates[0]) != 1 {
		t.Fatalf("updates %v", ma.updates)
	}
}

func TestDefaultBulkUpdateReportsProgressOnFailure(t *testing.T) {
	ma := newBulkAdmin("IsActive")
	ma.failOn = 2
	a, _ := GetAction(ma, BulkEditName)
	first, second := &bulkItem{ID: 1}, &bulkItem{ID: 2}
	_, err := a.Run(context.Background(), ma, []any{first, second}, map[string]any{"IsActive": false}, nil)
	var actionErr *ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("got %T %v", err, err)
	}
	if actionErr.Message != "Updated 1 of 2, then failed: boom" || len(actionErr.Done) != 1 || actionErr.Done[0] != first {
		t.Fatalf("got %+v", actionErr)
	}
}

type customBulkAdmin struct{ *bulkAdmin }

func (c customBulkAdmin) BulkUpdate(ctx context.Context, objects []any, data map[string]any, p *Principal) (string, error) {
	return "custom", nil
}

func TestBulkUpdaterOverridesTheLoop(t *testing.T) {
	ma := customBulkAdmin{newBulkAdmin("IsActive")}
	a, _ := GetAction(ma, BulkEditName)
	res, _ := a.Run(context.Background(), ma, []any{&bulkItem{ID: 1}}, map[string]any{"IsActive": false}, nil)
	if res.Message != "custom" || len(ma.updates) != 0 {
		t.Fatalf("got %+v", res)
	}
}

func TestValidateBulkEditFields(t *testing.T) {
	if err := ValidateBulkEditFields(newBulkAdmin("Nope")); err == nil || !strings.Contains(err.Error(), "Nope") {
		t.Fatalf("got %v", err)
	}
	ro := newBulkAdmin("Email")
	ro.ReadOnlyFieldNames = []string{"Email"}
	if err := ValidateBulkEditFields(ro); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("got %v", err)
	}
	if err := ValidateBulkEditFields(newBulkAdmin("IsActive")); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterPanicsOnBadBulkEditFields(t *testing.T) {
	mustPanic(t, "Nope", func() { New(WithModelAdmins(newBulkAdmin("Nope"))) })
}
