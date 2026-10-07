package core

import (
	"context"
	"fmt"
)

// BulkEditor is how the framework reads a ModelAdmin's bulk-editable
// fields; BaseModelAdmin implements it through BulkEditFieldNames. It is an
// optional interface so a ModelAdmin written from scratch need not grow a
// method.
type BulkEditor interface {
	BulkEditFields() []string
}

// BulkUpdater replaces the built-in bulk_edit's per-object Update loop,
// e.g. with one call to a backend's bulk endpoint. data holds only the
// fields the user ticked "Change" on.
type BulkUpdater interface {
	BulkUpdate(ctx context.Context, objects []any, data map[string]any, principal *Principal) (string, error)
}

func BulkEditFieldsOf(modelAdmin ModelAdmin) []string {
	if editor, ok := modelAdmin.(BulkEditor); ok {
		return editor.BulkEditFields()
	}
	return nil
}

// NewBulkEditAction is the built-in "Edit selected": a form action over
// the given fields that applies the ticked ones to every selected record
// through BulkUpdater when the ModelAdmin implements it, and through its
// own Update hook otherwise.
func NewBulkEditAction(fields []Field) Action {
	return Action{
		Name:       BulkEditName,
		Label:      N_("Edit selected"),
		Permission: "update",
		Where:      ActionWhereList,
		Form:       fields,
		FormHandler: func(ctx context.Context, modelAdmin ModelAdmin, objects []any, data map[string]any, principal *Principal) (ActionResult, error) {
			if updater, ok := modelAdmin.(BulkUpdater); ok {
				message, err := updater.BulkUpdate(ctx, objects, data, principal)
				return ActionResult{Message: message}, err
			}
			updated := 0
			for _, obj := range objects {
				patch := make(map[string]any, len(data))
				for k, v := range data {
					patch[k] = v
				}
				if _, err := modelAdmin.Update(ctx, obj, patch); err != nil {
					// Stop at the first failure and report how far it got, as
					// delete_selected does.
					return ActionResult{}, &ActionError{
						Message: T(ctx, "Updated %d of %d, then failed: %s", updated, len(objects), RecordErrorText(err)),
						Done:    objects[:updated],
						Err:     err,
					}
				}
				updated++
			}
			return ActionResult{Message: TN(ctx, "Updated %d record.", "Updated %d records.", updated, updated)}, nil
		},
	}
}

// ValidateBulkEditFields reports a bulk-edit field that is not a form
// field, or that is read-only.
func ValidateBulkEditFields(modelAdmin ModelAdmin) error {
	formFields := map[string]bool{}
	for _, name := range modelAdmin.FormFields() {
		formFields[name] = true
	}
	for _, name := range BulkEditFieldsOf(modelAdmin) {
		if !formFields[name] {
			return fmt.Errorf("%s: BulkEditFieldNames names %q, which is not one of its form fields", modelAdmin.Slug(), name)
		}
		if modelAdmin.IsReadOnly(name, nil) {
			return fmt.Errorf("%s: BulkEditFieldNames names %q, which is read-only", modelAdmin.Slug(), name)
		}
	}
	return nil
}
