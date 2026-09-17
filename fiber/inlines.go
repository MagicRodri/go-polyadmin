package fiber

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
)

func findInline(modelAdmin core.ModelAdmin, childSlug string) (core.Inline, bool) {
	for _, inline := range modelAdmin.Inlines() {
		if inline.Child == childSlug {
			return inline, true
		}
	}
	return core.Inline{}, false
}

// validateInlines runs the two startup invariants an Inline
// declaration must satisfy: no two inlines on one parent share a
// Child slug, and each Inline's FKField must be a ForeignKey/OneToOne
// field on the child whose Relation.Target is the parent's own slug.
// Run once per registered ModelAdmin at Mount time (after every
// ModelAdmin is registered, so cross-admin lookups resolve), before
// any request is served -- mirrors Register's duplicate-slug panic
// precedent, returned as an error here since Mount already returns one.
func validateInlines(admin *core.Admin, modelAdmin core.ModelAdmin) error {
	parentSlug := modelAdmin.Slug()
	seen := make(map[string]bool)
	for _, inline := range modelAdmin.Inlines() {
		if seen[inline.Child] {
			return fmt.Errorf("polyadmin: %q declares more than one inline for child %q", parentSlug, inline.Child)
		}
		seen[inline.Child] = true

		childAdmin, ok := admin.GetModelAdmin(inline.Child)
		if !ok {
			return fmt.Errorf("polyadmin: %q's inline references unknown child %q", parentSlug, inline.Child)
		}
		field, ok := childAdmin.Field(inline.FKField)
		if !ok || field.Relation == nil ||
			(field.Type != core.FieldTypeForeignKey && field.Type != core.FieldTypeOneToOne) ||
			field.Relation.Target != parentSlug {
			return fmt.Errorf(
				"polyadmin: %q's inline fk_field %q on child %q must be a ForeignKey/OneToOne field whose relation targets %q",
				parentSlug, inline.FKField, inline.Child, parentSlug,
			)
		}
	}
	return nil
}

func inlineLabel(admin *core.Admin, inline core.Inline) string {
	if inline.Label != "" {
		return inline.Label
	}
	childAdmin, _ := admin.GetModelAdmin(inline.Child)
	return childAdmin.VerboseName() + "s"
}

// excluding returns names without exclude -- used to drop an Inline's
// FKField (implied by context, never rendered as its own input) from
// the child's own FormFields/DetailFields.
func excluding(names []string, exclude string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != exclude {
			out = append(out, n)
		}
	}
	return out
}

// inlineMultiSelectRows is how many options a tabular inline's
// many-to-many listbox shows before it scrolls. Four keeps the row
// close to the height of the single-line controls beside it, which is
// what makes the table read as rows rather than as stacked blocks.
const inlineMultiSelectRows = 4

// relationOptions is relation.Options, or none when there is no relation.
func relationOptions(relation *relationFieldOptions) []relationOption {
	if relation == nil {
		return nil
	}
	return relation.Options
}

// inlineTableCellHTML is a trimmed sibling of formInputHTML for the
// tabular inline layout: a bare input/select, no <label>/wrapper <div>
// (the column header <th> already carries the label; a <form> can't
// wrap a table row/cell, so there's no per-field wrapper to put a
// label inside anyway). Deliberately duplicates formInputHTML's
// per-type branches rather than trying to strip its wrapper.
func (r *Renderer) inlineTableCellHTML(basePath string, field core.Field, value any, errs []string, relation *relationFieldOptions) template.HTML {
	name := html.EscapeString(field.Name)
	// Compact flavors of the same shadcn controls the full form uses.
	fieldClasses := classInputCompact
	selectClasses := classSelectCell

	var b strings.Builder
	switch field.Type {
	case core.FieldTypeBoolean:
		checked, _ := value.(bool)
		fmt.Fprintf(&b, `<input type="checkbox" name="%s" value="true" autocomplete="off" %s class="%s">`,
			name, attr(checked, "checked"), classCheckbox)

	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		// The same trigger-and-popover control the full form uses, in its
		// compact flavour: a native <select> here would be the one control
		// in the row that is not the admin's own.
		options := make([]fieldOptionData, 0, 1)
		options = append(options, fieldOptionData{Value: "", Label: "\u2014"})
		if relation != nil {
			for _, opt := range relation.Options {
				pk := fmt.Sprint(opt.PK)
				options = append(options, fieldOptionData{Value: pk, Label: opt.Label, Selected: pk == fmt.Sprint(relation.SelectedPK)})
			}
		}
		cell, err := r.uiHTML("ui/select", map[string]any{
			"name": field.Name, "options": options, "placeholder": "\u2014", "compact": true,
		})
		if err != nil {
			return template.HTML("")
		}
		b.WriteString(string(cell))

	case core.FieldTypeManyToMany:
		// The same shadcn control the full form uses: a trigger with chips
		// and a portalled popover, so a row stays one line tall however
		// many options there are -- the native <select multiple> it
		// replaces had to be capped and scrolled internally to avoid
		// 195px rows.
		options := make([]fieldOptionData, 0, len(relationOptions(relation)))
		for _, opt := range relationOptions(relation) {
			pk := fmt.Sprint(opt.PK)
			selected := false
			for _, spk := range relation.SelectedPKs {
				if fmt.Sprint(spk) == pk {
					selected = true
					break
				}
			}
			options = append(options, fieldOptionData{Value: pk, Label: opt.Label, Selected: selected})
		}
		cell, err := r.uiHTML("ui/multi-select", map[string]any{
			"name": field.Name, "options": options, "placeholder": r.t("Select…"), "compact": true,
		})
		if err != nil {
			return template.HTML("")
		}
		b.WriteString(string(cell))

	case core.FieldTypeEnum:
		fmt.Fprintf(&b, `<select name="%s" autocomplete="off" class="%s">`, name, selectClasses)
		for _, choice := range field.Choices {
			choiceStr := fmt.Sprint(choice)
			selected := choiceStr == stringOrEmpty(value)
			fmt.Fprintf(&b, `<option value="%s" %s>%s</option>`, html.EscapeString(choiceStr), attr(selected, "selected"), html.EscapeString(choiceStr))
		}
		b.WriteString(`</select>`)

	default:
		fmt.Fprintf(&b, `<input type="%s" name="%s" value="%s" autocomplete="off" class="%s">`,
			inputTypeFor(field.Type), name, html.EscapeString(inputValue(field.Type, value)), fieldClasses)
	}
	for _, e := range errs {
		fmt.Fprintf(&b, `<p class="mt-0.5 %s">%s</p>`, classTextError, html.EscapeString(e))
	}
	_ = basePath // no autocomplete combobox in the compact tabular cell -- see docs/inlines.md's known limitations
	return template.HTML(b.String())
}

// inlineDetailRowHTML wraps a read-only value with its own label, for
// the stacked read-only layout's per-child <dl>-style rows (the
// tabular read-only layout doesn't need this -- its <th> header
// already carries the label, so its cells stay bare fieldValueHTML).
func inlineDetailRowHTML(label string, valueHTML template.HTML) template.HTML {
	return template.HTML(fmt.Sprintf(
		`<div class="flex justify-between gap-4 py-1 text-sm"><dt class="text-muted-foreground">%s</dt><dd class="text-right">%s</dd></div>`,
		html.EscapeString(label), valueHTML,
	))
}
