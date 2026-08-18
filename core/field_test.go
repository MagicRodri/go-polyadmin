package core

import (
	"errors"
	"reflect"
	"testing"
)

type testUser struct {
	Email    string
	Nickname string
}

func TestGetValueReadsStructField(t *testing.T) {
	f := NewField("Email", FieldTypeString)
	got := f.GetValue(testUser{Email: "john@example.com"})
	if got != "john@example.com" {
		t.Fatalf("got %v", got)
	}
}

func TestGetValueReadsMapKey(t *testing.T) {
	f := NewField("email", FieldTypeString)
	got := f.GetValue(map[string]any{"email": "john@example.com"})
	if got != "john@example.com" {
		t.Fatalf("got %v", got)
	}
}

func TestGetValueFallsBackToDefault(t *testing.T) {
	// A struct field always exists (with its zero value), so "missing"
	// is only meaningful for a map key that was never set.
	f := NewField("nickname", FieldTypeString, WithDefault("anon"))
	got := f.GetValue(map[string]any{})
	if got != "anon" {
		t.Fatalf("got %v, want anon", got)
	}
}

func TestGetValueMissingWithoutDefaultIsNil(t *testing.T) {
	f := NewField("missing", FieldTypeString)
	if got := f.GetValue(map[string]any{}); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestGetValueUsesCustomGetter(t *testing.T) {
	f := NewField("Email", FieldTypeString, WithGetter(func(obj any) any { return "overridden" }))
	got := f.GetValue(testUser{Email: "john@example.com"})
	if got != "overridden" {
		t.Fatalf("got %v", got)
	}
}

func TestDefaultLabelFromName(t *testing.T) {
	f := NewField("created_at", FieldTypeDateTime)
	if f.Label != "Created At" {
		t.Fatalf("got %q", f.Label)
	}
}

func TestRequiredValidation(t *testing.T) {
	f := NewField("IsActive", FieldTypeBoolean, WithRequired())
	if errs := f.Validate(nil); !reflect.DeepEqual(errs, []string{"Is Active is required."}) {
		t.Fatalf("got %v", errs)
	}
	if errs := f.Validate(true); errs != nil {
		t.Fatalf("got %v, want none", errs)
	}
}

func TestCustomValidator(t *testing.T) {
	notAdmin := func(value any) error {
		if value == "admin" {
			return errors.New("reserved username.")
		}
		return nil
	}
	f := NewField("Username", FieldTypeString, WithValidators(notAdmin))
	if errs := f.Validate("admin"); !reflect.DeepEqual(errs, []string{"reserved username."}) {
		t.Fatalf("got %v", errs)
	}
	if errs := f.Validate("john"); errs != nil {
		t.Fatalf("got %v, want none", errs)
	}
}

func TestEnumFieldChoices(t *testing.T) {
	f := NewField("Role", FieldTypeEnum, WithChoices("admin", "member"))
	if !reflect.DeepEqual(f.Choices, []any{"admin", "member"}) {
		t.Fatalf("got %v", f.Choices)
	}
}
