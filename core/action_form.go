package core

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// BulkEditName is the built-in bulk edit's action name, reserved like
// DeleteSelectedName. See docs/bulk-edit.md.
const BulkEditName = "bulk_edit"

// Download is a file an action answers with instead of a flash message:
// Content, or Stream for a body read as it is sent. A nil Content with no
// Stream is an empty file.
type Download struct {
	Filename    string
	ContentType string
	Content     []byte
	Stream      io.Reader
}

func (d Download) Validate() error {
	if d.Content != nil && d.Stream != nil {
		return errors.New("polyadmin: a Download takes Content or Stream, not both")
	}
	return nil
}

// ActionResult is what a form or download action's handler returns: a
// flash message, or a file to send instead.
type ActionResult struct {
	Message  string
	Download *Download
}

// ActionFormHandler runs a form action with the validated form values.
type ActionFormHandler func(ctx context.Context, modelAdmin ModelAdmin, objects []any, data map[string]any, principal *Principal) (ActionResult, error)

// ActionDownloadHandler runs an action that may answer with a file, for
// actions that ask for no input first.
type ActionDownloadHandler func(ctx context.Context, modelAdmin ModelAdmin, objects []any, principal *Principal) (ActionResult, error)

// ActionFormError, returned by a form action's handler, redisplays the
// form with Errors. The "" key holds messages that belong to no field.
type ActionFormError struct {
	Errors map[string][]string
}

func (e *ActionFormError) Error() string { return "polyadmin: action form rejected" }

// WithActionForm makes the action ask for these fields on a page of its
// own before running; pair it with WithActionFormHandler.
func WithActionForm(fields ...Field) func(*Action) {
	return func(a *Action) { a.Form = append([]Field{}, fields...) }
}

func WithActionSubmitLabel(label string) func(*Action) {
	return func(a *Action) { a.SubmitLabel = label }
}

func WithActionFormHandler(handler ActionFormHandler) func(*Action) {
	return func(a *Action) { a.FormHandler = handler }
}

func WithActionDownloadHandler(handler ActionDownloadHandler) func(*Action) {
	return func(a *Action) { a.DownloadHandler = handler }
}

func (a Action) HasForm() bool { return a.FormHandler != nil }

// Run calls whichever handler the action carries; data is nil for an
// action without a form.
func (a Action) Run(ctx context.Context, modelAdmin ModelAdmin, objects []any, data map[string]any, principal *Principal) (ActionResult, error) {
	switch {
	case a.FormHandler != nil:
		return a.FormHandler(ctx, modelAdmin, objects, data, principal)
	case a.DownloadHandler != nil:
		return a.DownloadHandler(ctx, modelAdmin, objects, principal)
	default:
		message, err := a.Handler(ctx, modelAdmin, objects, principal)
		return ActionResult{Message: message}, err
	}
}

func validateAction(a Action) error {
	handlers := 0
	for _, set := range []bool{a.Handler != nil, a.FormHandler != nil, a.DownloadHandler != nil} {
		if set {
			handlers++
		}
	}
	switch {
	case handlers == 0:
		return fmt.Errorf("action %q has no handler", a.Name)
	case (len(a.Form) > 0) != (a.FormHandler != nil):
		return fmt.Errorf("action %q: WithActionForm and WithActionFormHandler go together", a.Name)
	case handlers > 1:
		return fmt.Errorf("action %q sets more than one handler", a.Name)
	case len(a.Form) > 0 && a.Confirm != "":
		return fmt.Errorf("action %q: a form action cannot also take a confirm prompt; the form page is the confirmation", a.Name)
	}
	return nil
}
