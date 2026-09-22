package core

import (
	"context"
	"fmt"
)

// ActionHandler runs an Action over the resolved objects. The returned
// string, if non-empty, becomes the flash message; principal is nil
// when the Admin has no Authenticator configured.
type ActionHandler func(ctx context.Context, modelAdmin ModelAdmin, objects []any, principal *Principal) (string, error)

// ActionWhere says which pages offer an Action. The zero value behaves as
// ActionWhereBoth, so an Action literal that never mentions it keeps its
// old behaviour.
type ActionWhere string

const (
	ActionWhereBoth   ActionWhere = "both"
	ActionWhereList   ActionWhere = "list"
	ActionWhereDetail ActionWhere = "detail"
)

func (w ActionWhere) onList() bool   { return w != ActionWhereDetail }
func (w ActionWhere) onDetail() bool { return w != ActionWhereList }

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
	// dialog in templates/admin/components/action_confirm_modal.html) --
	// "" means no confirmation.
	Confirm string
	// Permission is an extra permission suffix checked via
	// ResourcePermission(slug, Permission) alongside the resource's
	// "view" permission -- "" means no extra check.
	Permission string
	// Where places the action: the list page's bulk bar, a record's detail
	// page, or both. Placement only, not authorization -- the action route
	// serves every action whichever page offered it, and checks Permission
	// there.
	Where ActionWhere
}

func NewAction(name string, handler ActionHandler, opts ...func(*Action)) Action {
	a := Action{Name: name, Label: defaultLabel(name), Handler: handler, Where: ActionWhereBoth}
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

func WithActionWhere(where ActionWhere) func(*Action) {
	return func(a *Action) {
		switch where {
		case ActionWhereBoth, ActionWhereList, ActionWhereDetail:
			a.Where = where
		default:
			panic(fmt.Sprintf("polyadmin: action %q: unknown placement %q", a.Name, where))
		}
	}
}

// DeleteSelectedName is the built-in bulk delete's action name. It is
// reserved: a ModelAdmin that declares an Action of the same name
// replaces the built-in rather than colliding with it, which is how you
// customise the confirmation text or the deletion itself.
const DeleteSelectedName = "delete_selected"

// NewDeleteSelectedAction is the bulk delete every admin gets for free
// -- the single most common action anyone would otherwise write by
// hand.
//
// It is expressed entirely in terms of the ModelAdmin's own Delete
// hook, so it works against whatever storage the application actually
// has and honours whatever that hook already does (cascades, soft
// deletes, hooks of its own).
//
// Permission "delete", not the resource's bare "view": the action route
// checks it on top, so a principal who may look at a list but not
// destroy its rows is refused -- and, because the same check drives the
// listbox, never offered it in the first place.
func NewDeleteSelectedAction() Action {
	return Action{
		Name:       DeleteSelectedName,
		Label:      N_("Delete selected"),
		Confirm:    N_("Delete the selected records? This cannot be undone."),
		Permission: "delete",
		Where:      ActionWhereList,
		Handler: func(ctx context.Context, modelAdmin ModelAdmin, objects []any, principal *Principal) (string, error) {
			deleted := 0
			for _, obj := range objects {
				if err := modelAdmin.Delete(ctx, obj); err != nil {
					// Stop at the first failure and report how far it
					// got: silently continuing would leave the user
					// unable to tell which records survived.
					return "", fmt.Errorf("%s: %w", T(ctx, "Deleted %d of %d, then failed", deleted, len(objects)), err)
				}
				deleted++
			}
			return TN(ctx, "Deleted %d record.", "Deleted %d records.", deleted, deleted), nil
		},
	}
}

func GetAction(modelAdmin ModelAdmin, name string) (Action, bool) {
	for _, action := range modelAdmin.Actions() {
		if action.Name == name {
			return action, true
		}
	}
	return Action{}, false
}

// ActionsForList is what the list page's bulk bar offers.
func ActionsForList(modelAdmin ModelAdmin) []Action {
	var out []Action
	for _, action := range modelAdmin.Actions() {
		if action.Where.onList() {
			out = append(out, action)
		}
	}
	return out
}

// ActionsForDetail is what one record's detail page offers. delete_selected
// is a bulk action, so it is never among them -- not by Where, not by being
// named in DetailActions, not when a ModelAdmin replaces it.
func ActionsForDetail(modelAdmin ModelAdmin) []Action {
	var candidates []Action
	for _, action := range modelAdmin.Actions() {
		if action.Name != DeleteSelectedName {
			candidates = append(candidates, action)
		}
	}
	names := modelAdmin.DetailActions()
	var out []Action
	if names == nil {
		for _, action := range candidates {
			if action.Where.onDetail() {
				out = append(out, action)
			}
		}
		return out
	}
	for _, name := range names {
		for _, action := range candidates {
			if action.Name == name {
				out = append(out, action)
				break
			}
		}
	}
	return out
}

// ValidateDetailActions reports a DetailActions name that is not one of the
// ModelAdmin's actions.
func ValidateDetailActions(modelAdmin ModelAdmin) error {
	known := map[string]bool{}
	for _, action := range modelAdmin.Actions() {
		known[action.Name] = true
	}
	for _, name := range modelAdmin.DetailActions() {
		if !known[name] {
			return fmt.Errorf("%s: DetailActions names %q, which is not one of its actions", modelAdmin.Slug(), name)
		}
	}
	return nil
}
