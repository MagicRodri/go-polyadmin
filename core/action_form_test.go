package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func noopForm(ctx context.Context, ma ModelAdmin, objects []any, data map[string]any, p *Principal) (ActionResult, error) {
	return ActionResult{Message: "ran:" + data["reason"].(string)}, nil
}

func mustPanic(t *testing.T, contains string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		s, _ := r.(string)
		if r == nil || !strings.Contains(s, contains) {
			t.Fatalf("expected panic containing %q, got %v", contains, r)
		}
	}()
	fn()
}

func TestFormActionRunsTheFormHandlerWithData(t *testing.T) {
	a := NewAction("move", nil, WithActionForm(NewField("reason", FieldTypeString)), WithActionFormHandler(noopForm), WithActionSubmitLabel("Move"))
	if !a.HasForm() || a.SubmitLabel != "Move" || len(a.Form) != 1 {
		t.Fatalf("form not recorded: %+v", a)
	}
	res, err := a.Run(context.Background(), nil, nil, map[string]any{"reason": "x"}, nil)
	if err != nil || res.Message != "ran:x" {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestPlainActionRunWrapsTheMessage(t *testing.T) {
	a := NewAction("plain", func(ctx context.Context, ma ModelAdmin, objects []any, p *Principal) (string, error) {
		return "done", nil
	})
	res, err := a.Run(context.Background(), nil, nil, nil, nil)
	if err != nil || res.Message != "done" || a.HasForm() {
		t.Fatalf("got %+v %v", res, err)
	}
}

func TestDownloadActionRunsTheDownloadHandler(t *testing.T) {
	a := NewAction("export", nil, WithActionDownloadHandler(func(ctx context.Context, ma ModelAdmin, objects []any, p *Principal) (ActionResult, error) {
		return ActionResult{Download: &Download{Filename: "a.csv", Content: []byte("x")}}, nil
	}))
	res, _ := a.Run(context.Background(), nil, nil, nil, nil)
	if res.Download == nil || res.Download.Filename != "a.csv" {
		t.Fatalf("got %+v", res)
	}
}

func TestNewActionRejectsBadCombinations(t *testing.T) {
	plain := func(ctx context.Context, ma ModelAdmin, o []any, p *Principal) (string, error) { return "", nil }
	download := func(ctx context.Context, ma ModelAdmin, o []any, p *Principal) (ActionResult, error) {
		return ActionResult{}, nil
	}
	mustPanic(t, "no handler", func() { NewAction("x", nil) })
	mustPanic(t, "go together", func() { NewAction("x", nil, WithActionFormHandler(noopForm)) })
	mustPanic(t, "go together", func() { NewAction("x", plain, WithActionForm(NewField("a", FieldTypeString))) })
	mustPanic(t, "confirm", func() {
		NewAction("x", nil, WithActionForm(NewField("a", FieldTypeString)), WithActionFormHandler(noopForm), WithActionConfirm("Sure?"))
	})
	mustPanic(t, "more than one handler", func() { NewAction("x", plain, WithActionDownloadHandler(download)) })
}

func TestDownloadValidateRejectsTwoBodies(t *testing.T) {
	if (Download{Filename: "a", Content: []byte("x"), Stream: strings.NewReader("x")}).Validate() == nil {
		t.Fatal("two bodies accepted")
	}
	if err := (Download{Filename: "a"}).Validate(); err != nil {
		t.Fatalf("empty download rejected: %v", err)
	}
	if err := (Download{Filename: "a", Stream: strings.NewReader("x")}).Validate(); err != nil {
		t.Fatalf("stream rejected: %v", err)
	}
}

func TestActionFormErrorIsDetectableWithErrorsAs(t *testing.T) {
	var err error = &ActionFormError{Errors: map[string][]string{"reason": {"bad"}}}
	var formErr *ActionFormError
	if !errors.As(err, &formErr) || formErr.Errors["reason"][0] != "bad" {
		t.Fatal("errors.As failed")
	}
}
