package core

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestRecordFormErrorTextPutsNonFieldErrorsFirstThenFieldsSorted(t *testing.T) {
	err := &RecordFormError{Errors: map[string][]string{
		"Slug":  {"taken"},
		"":      {"Conflict.", "Try again."},
		"Email": {"bad", "worse"},
	}}
	want := "Conflict.; Try again.; bad; worse; taken"
	if err.Text() != want || err.Error() != want {
		t.Fatalf("Text %q Error %q", err.Text(), err.Error())
	}
}

func TestRecordErrorTextUnwrapsARecordFormError(t *testing.T) {
	wrapped := fmt.Errorf("save: %w", &RecordFormError{Errors: map[string][]string{"": {"Still referenced."}}})
	if got := RecordErrorText(wrapped); got != "Still referenced." {
		t.Fatalf("got %q", got)
	}
	if got := RecordErrorText(errors.New("disk full")); got != "disk full" {
		t.Fatalf("got %q", got)
	}
}

func TestActionErrorErrorIsItsMessage(t *testing.T) {
	err := &ActionError{Message: "Nope.", Level: "warning"}
	if err.Error() != "Nope." {
		t.Fatalf("got %q", err.Error())
	}
}

func TestBuiltInBulkFailuresKeepTheirCause(t *testing.T) {
	cause := errors.New("disk full")
	_, err := NewDeleteSelectedAction().Handler(context.Background(), causeDeleteAdmin{cause: cause}, []any{"a"}, nil)
	if !errors.Is(err, cause) {
		t.Fatalf("delete: %v does not wrap its cause", err)
	}
	ma := newBulkAdmin("IsActive")
	ma.failOn = 1
	a, _ := GetAction(ma, BulkEditName)
	_, err = a.Run(context.Background(), ma, []any{&bulkItem{ID: 1}}, map[string]any{"IsActive": false}, nil)
	var actionErr *ActionError
	if !errors.As(err, &actionErr) || actionErr.Unwrap() == nil || actionErr.Unwrap().Error() != "boom" {
		t.Fatalf("bulk edit: %v does not wrap its cause", err)
	}
}

type causeDeleteAdmin struct {
	BaseModelAdmin
	cause error
}

func (a causeDeleteAdmin) Delete(ctx context.Context, obj any) error { return a.cause }
