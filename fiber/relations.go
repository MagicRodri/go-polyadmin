package fiber

import (
	"context"

	"github.com/MagicRodri/go-polyadmin/core"
)

func relationFields(modelAdmin core.ModelAdmin, names []string) []core.Field {
	var out []core.Field
	for _, name := range names {
		field, ok := modelAdmin.Field(name)
		if ok && field.Relation != nil {
			out = append(out, field)
		}
	}
	return out
}

// computeRelationPermissions reports, for every relation field named
// in `names`, whether the current principal may view that field's
// target resource -- the admin must not link to an object
// the principal isn't authorized to see.
func computeRelationPermissions(admin *core.Admin, principal *core.Principal, modelAdmin core.ModelAdmin, names []string) map[string]bool {
	result := make(map[string]bool)
	for _, field := range relationFields(modelAdmin, names) {
		target := field.Relation.Target
		if _, seen := result[target]; seen {
			continue
		}
		targetAdmin, ok := admin.GetModelAdmin(target)
		switch {
		case !ok || !targetAdmin.CanView():
			result[target] = false
		case admin.Authorizer == nil:
			result[target] = true
		default:
			result[target] = admin.Authorizer.Can(principal, core.ResourcePermission(target, "view"), targetAdmin)
		}
	}
	return result
}

// computeRelationOptions builds the selectable options for each
// relation field in modelAdmin.FormFields(). A field named in
// modelAdmin.AutocompleteFields() skips loading the target's queryset
// entirely and only resolves the current selection's own label -- the
// rest of the options are fetched on demand from the /lookup route
// as the user types, via the combobox branch of
// formInputHTML. Every other relation field keeps populating a
// same-page <select> from the target's full queryset, gated only by
// its static CanView -- coarser than computeRelationPermissions, fine
// for a same-page selector.
func computeRelationOptions(admin *core.Admin, modelAdmin core.ModelAdmin, obj any) map[string]*relationFieldOptions {
	autocompleteNames := make(map[string]bool)
	for _, name := range modelAdmin.AutocompleteFields() {
		autocompleteNames[name] = true
	}

	result := make(map[string]*relationFieldOptions)
	for _, field := range relationFields(modelAdmin, modelAdmin.FormFields()) {
		targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
		if !ok || !targetAdmin.CanView() {
			continue
		}
		displayField, _ := targetAdmin.Field(field.Relation.DisplayField)

		if autocompleteNames[field.Name] && (field.Type == core.FieldTypeForeignKey || field.Type == core.FieldTypeOneToOne) {
			opts := &relationFieldOptions{Autocomplete: true, LookupTarget: field.Relation.Target}
			if obj != nil {
				if current := field.GetValue(obj); !core.IsNil(current) {
					opts.SelectedPK = targetAdmin.GetPK(current)
					opts.SelectedLabel = fmtValue(displayField.GetValue(current))
				}
			}
			result[field.Name] = opts
			continue
		}

		queryset, _ := targetAdmin.GetQueryset(context.Background())
		// The Fiber adapter expects GetQueryset to return []any --
		// ModelAdmins used with it should return that, not a concrete
		// []T (Go doesn't implicitly convert between the two).
		items, _ := queryset.([]any)
		options := make([]relationOption, 0, len(items))
		for _, related := range items {
			options = append(options, relationOption{PK: targetAdmin.GetPK(related), Label: fmtValue(displayField.GetValue(related))})
		}

		opts := &relationFieldOptions{Options: options}
		if obj != nil {
			current := field.GetValue(obj)
			if field.Type == core.FieldTypeManyToMany {
				currentItems, _ := current.([]any)
				for _, item := range currentItems {
					opts.SelectedPKs = append(opts.SelectedPKs, targetAdmin.GetPK(item))
				}
			} else if !core.IsNil(current) {
				opts.SelectedPK = targetAdmin.GetPK(current)
			}
		}
		result[field.Name] = opts
	}
	return result
}

func fmtValue(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	return stringOrEmpty(value)
}
