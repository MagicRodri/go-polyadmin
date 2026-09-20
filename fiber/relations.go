package fiber

import (
	"context"
	"fmt"

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

		// Unlimited: a non-autocomplete relation renders every choice
		// inline, which is exactly what this widget is for. Going
		// through listObjects means a ListQuerier target answers this
		// from its own data source like every other list query.
		//
		// (The Fiber adapter expects GetQueryset to return []any --
		// ModelAdmins used with it should return that, not a concrete
		// []T; Go doesn't implicitly convert between the two.)
		items, _, _ := core.ListObjects(context.Background(), targetAdmin, core.ListRequest{Unlimited: true})
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

// relationFilterChoices is a relation filter's choice list: every record
// of the target ModelAdmin, as (pk, display) pairs.
//
// ok is false when the principal may not view the target. The caller
// drops the filter entirely in that case -- an empty filter group reads
// as a broken control, and a reader who may not see organizations should
// not be told they exist. This is the principal-aware check, not
// computeRelationOptions' coarser CanView: a filter offers records by
// name, so it has to answer to the same authorizer the target's own list
// does.
//
// The queryset is loaded whole and uncapped, through listObjects and
// context.Background(), exactly as computeRelationOptions already does
// for a non-autocomplete relation <select>. AutocompleteFields is the
// answer to a large target, and it is the same answer in both places; a
// cap here and not there would have the panel and the form disagree
// about the same relation.
func relationFilterChoices(
	admin *core.Admin,
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	field core.Field,
) ([]filterChoice, bool) {
	if field.Relation == nil {
		return nil, false
	}
	allowed := computeRelationPermissions(admin, principal, modelAdmin, []string{field.Name})
	if !allowed[field.Relation.Target] {
		return nil, false
	}
	targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
	if !ok {
		return nil, false
	}
	objects, _, err := core.ListObjects(context.Background(), targetAdmin, core.ListRequest{Unlimited: true})
	if err != nil {
		return nil, false
	}
	displayField, hasDisplay := targetAdmin.Field(field.Relation.DisplayField)
	choices := make([]filterChoice, 0, len(objects))
	for _, obj := range objects {
		pk := fmt.Sprint(targetAdmin.GetPK(obj))
		label := pk
		if hasDisplay {
			label = fmtValue(displayField.GetValue(obj))
		}
		choices = append(choices, filterChoice{Value: pk, Label: label})
	}
	return choices, true
}

// autocompleteFields is AutocompleteFields() as a set, for the lookups
// the control assembly does per filter.
func autocompleteFields(modelAdmin core.ModelAdmin) map[string]bool {
	names := make(map[string]bool)
	for _, name := range modelAdmin.AutocompleteFields() {
		names[name] = true
	}
	return names
}

// relationFilterCombobox resolves what the panel's combobox needs for a
// relation filter: the target's lookup route, and the label of whatever
// is currently selected. It loads no queryset -- that is the whole point
// of AutocompleteFields -- but the target must still be viewable, so the
// same permission check the link list uses applies here too.
//
// current is the filter's raw value, which is the target's primary key.
func relationFilterCombobox(
	admin *core.Admin,
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	field core.Field,
	current string,
	basePath string,
) (usesCombobox, viewable bool, lookupURL, selectedPK, selectedLabel string) {
	if field.Relation == nil {
		return false, false, "", "", ""
	}
	allowed := computeRelationPermissions(admin, principal, modelAdmin, []string{field.Name})
	if !allowed[field.Relation.Target] {
		return false, false, "", "", ""
	}
	targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
	if !ok {
		return false, false, "", "", ""
	}
	lookupURL = basePath + "/" + field.Relation.Target + "/lookup"
	if current != "" {
		// One object, not the queryset: the trigger has to show what is
		// selected or the reader cannot tell what they are filtering by.
		if related, err := targetAdmin.GetObject(context.Background(), current); err == nil && !core.IsNil(related) {
			selectedPK = current
			selectedLabel = current
			if displayField, hasDisplay := targetAdmin.Field(field.Relation.DisplayField); hasDisplay {
				selectedLabel = fmtValue(displayField.GetValue(related))
			}
		}
	}
	return true, true, lookupURL, selectedPK, selectedLabel
}
