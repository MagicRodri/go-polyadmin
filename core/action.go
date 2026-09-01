package core

import "context"

// ActionHandler runs an Action over the resolved objects. The returned
// string, if non-empty, becomes the flash message; principal is nil
// when the Admin has no Authenticator configured.
type ActionHandler func(ctx context.Context, modelAdmin ModelAdmin, objects []any, principal *Principal) (string, error)

// Action is a ModelAdmin capability applied to one or more records.
// It's invoked with a list of objects either way -- a
// "record" action from the detail page passes a list of exactly one, a
// "bulk" action from the list view's row-selection passes as many as
// were checked. There's deliberately no separate record/bulk type: the
// same Action definition serves both, since the handler never needs to
// know which UI entry point it was invoked from.
type Action struct {
	Name    string
	Label   string
	Handler ActionHandler
	// Confirm is a confirmation prompt shown before running (the
	// PinesUI modal in templates/admin/components/action_confirm_modal.html) --
	// "" means no confirmation.
	Confirm string
	// Permission is an extra permission suffix checked via
	// ResourcePermission(slug, Permission) alongside the resource's
	// "view" permission -- "" means no extra check.
	Permission string
}

func NewAction(name string, handler ActionHandler, opts ...func(*Action)) Action {
	a := Action{Name: name, Label: defaultLabel(name), Handler: handler}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

func WithActionLabel(label string) func(*Action) {
	return func(a *Action) { a.Label = label }
}

func WithActionConfirm(confirm string) func(*Action) {
	return func(a *Action) { a.Confirm = confirm }
}

func WithActionPermission(permission string) func(*Action) {
	return func(a *Action) { a.Permission = permission }
}

// GetAction finds a ModelAdmin's declared Action by name.
func GetAction(modelAdmin ModelAdmin, name string) (Action, bool) {
	for _, action := range modelAdmin.Actions() {
		if action.Name == name {
			return action, true
		}
	}
	return Action{}, false
}
