package fiber

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
)

// mustUI resolves a ui.go class string or panics. Only ever called from
// package-level var initializers below, so an unknown component or
// modifier fails at program start rather than mid-request -- the same
// loud-failure guarantee uiClasses gives templates, moved to init time
// for the class strings this file bakes in.
func mustUI(component string, modifiers ...string) string {
	classes, err := uiClasses(component, modifiers...)
	if err != nil {
		panic(err)
	}
	return classes
}

// The shadcn-derived class strings this file emits directly in Go,
// for the read-only value renderer (fieldValueHTML) and the compact
// tabular inline cell renderer (inlineTableCellHTML) -- both
// deliberately still build HTML by hand rather than through a
// template (see each's own doc comment for why). The full form-field
// renderer does not use these: it delegates to the ui/field template
// partial instead -- see formInputHTML.
var (
	classInputCompact = mustUI("input", "size-sm")
	classSelectSmall  = mustUI("select", "size-sm")
	classSelectAuto   = mustUI("select", "size-auto")
	classCheckbox     = mustUI("checkbox")
	classTextError    = mustUI("text", "error")

	classPlaceholder = mustUI("text", "placeholder")
	classLink        = mustUI("text", "link")
)

// fieldValueHTML renders a field's read-only value (list/detail),
// including permission-aware relation links. Built in
// Go rather than inside a template so every dynamic bit of text goes
// through html.EscapeString explicitly -- template.HTML bypasses
// html/template's auto-escaping, so this function is the one place
// that has to get escaping right by hand.
func fieldValueHTML(admin *core.Admin, basePath string, relationPermissions map[string]bool, field core.Field, value any) template.HTML {
	dash := template.HTML(`<span class="` + classPlaceholder + `">&mdash;</span>`)
	if core.IsNil(value) {
		return dash
	}
	switch field.Type {
	case core.FieldTypeBoolean:
		if b, _ := value.(bool); b {
			// No shadcn "success" token to defer to, so this picks an
			// emerald pair that clears contrast against bg-card in both
			// themes.
			return boolIconHTML("check", "text-emerald-600 dark:text-emerald-400", "Yes")
		}
		return boolIconHTML("close", classPlaceholder, "No")
	case core.FieldTypePassword:
		return template.HTML(`<span class="` + classPlaceholder + `">&bull;&bull;&bull;&bull;&bull;&bull;&bull;&bull;</span>`)
	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		return relatedLinkHTML(admin, basePath, relationPermissions, field.Relation, value)
	case core.FieldTypeManyToMany:
		items, _ := value.([]any)
		if len(items) == 0 {
			return dash
		}
		parts := make([]string, len(items))
		for i, item := range items {
			parts[i] = string(relatedLinkHTML(admin, basePath, relationPermissions, field.Relation, item))
		}
		return template.HTML(strings.Join(parts, ", "))
	default:
		return template.HTML(html.EscapeString(fmt.Sprint(value)))
	}
}

// boolIconHTML renders a boolean as a check or a cross rather than the
// words "Yes"/"No" -- Django admin's convention, and the one thing that
// makes a column of booleans scannable: a glyph reads as a shape at a
// glance where two similar-length words have to be read.
//
// The word stays in the markup as an sr-only label, so a screen reader
// still hears "Yes"/"No" and nothing depends on the icon alone (the
// icon itself is aria-hidden, from iconHTML). Exports are untouched --
// they stringify through core/exporter.go, never through here.
//
// `class` and `label` are package-internal constants, never field data,
// so neither needs escaping the way the value branches above do.
func boolIconHTML(icon, class, label string) template.HTML {
	return template.HTML(`<span class="inline-flex items-center ` + class + `">` +
		string(iconHTML(icon, "size-4")) +
		`<span class="sr-only">` + label + `</span></span>`)
}

func relatedLinkHTML(admin *core.Admin, basePath string, relationPermissions map[string]bool, relation *core.Relation, value any) template.HTML {
	if relation == nil || admin == nil {
		return template.HTML(html.EscapeString(fmt.Sprint(value)))
	}
	targetAdmin, ok := admin.GetModelAdmin(relation.Target)
	if !ok {
		return template.HTML(html.EscapeString(fmt.Sprint(value)))
	}
	displayField, _ := targetAdmin.Field(relation.DisplayField)
	label := html.EscapeString(fmt.Sprint(displayField.GetValue(value)))
	if relationPermissions == nil || !relationPermissions[relation.Target] {
		return template.HTML(label)
	}
	pk := html.EscapeString(fmt.Sprint(targetAdmin.GetPK(value)))
	href := html.EscapeString(fmt.Sprintf("%s/%s/%s", basePath, relation.Target, pk))
	return template.HTML(fmt.Sprintf(`<a class="%s" href="%s">%s</a>`, classLink, href, label))
}

// relationOption is one selectable choice in a relation <select>.
type relationOption struct {
	PK    any
	Label string
}

// relationFieldOptions is what a relation form field needs to render
// its <select>: the choices and which are currently selected.
type relationFieldOptions struct {
	Options     []relationOption
	SelectedPK  any
	SelectedPKs []any
	// Autocomplete/SelectedLabel/LookupTarget drive the lookup-driven
	// combobox branch of ui/field.html in place of the plain <select>
	// above -- see core.BaseModelAdmin.AutocompleteFieldNames.
	Autocomplete  bool
	SelectedLabel string
	LookupTarget  string
}

func attr(present bool, name string) string {
	if present {
		return name
	}
	return ""
}

func inputTypeFor(fieldType core.FieldType) string {
	switch fieldType {
	case core.FieldTypeInteger, core.FieldTypeDecimal:
		return "number"
	case core.FieldTypeEmail:
		return "email"
	case core.FieldTypeURL:
		return "url"
	case core.FieldTypeDate:
		return "date"
	case core.FieldTypeDateTime:
		return "datetime-local"
	case core.FieldTypePassword:
		return "password"
	default:
		return "text"
	}
}

// fieldOptionData is one <option> in a select-like control, resolved
// in Go -- see formInputHTML's doc comment for why comparisons and
// stringification never happen inside ui/field.html itself.
type fieldOptionData struct {
	Value    string
	Label    string
	Selected bool
}

// formInputHTML renders a field's form input as the Form/FormItem unit
// ported from shadcn/ui -- label, control, description, error -- by
// resolving every per-type value (stringification, selection matching,
// option lists) here in Go and handing the result to the ui/field
// template partial to dispatch and print. The Go counterpart to
// fieldValueHTML; the same care around correct escaping applies,
// though here it's html/template's own contextual auto-escaping doing
// the work rather than a manual html.EscapeString call, since every
// value reaches the template as a plain field on the data map instead
// of a hand-built HTML string.
//
// A method on Renderer only because rendering goes through r.uiHTML,
// which executes against the shared ui/*.html template set (see
// NewRenderer's r.uiSet).
func (r *Renderer) formInputHTML(basePath string, field core.Field, value any, errs []string, relation *relationFieldOptions) (template.HTML, error) {
	data := map[string]any{
		"Name":        field.Name,
		"Label":       field.Label,
		"Required":    field.Required,
		"ReadOnly":    field.ReadOnly,
		"Disabled":    field.Disabled,
		"Placeholder": field.Placeholder,
		"HelpText":    field.HelpText,
		"Type":        string(field.Type),
		"InputType":   inputTypeFor(field.Type),
		"StringValue": stringOrEmpty(value),
		"Errors":      errs,
		"BasePath":    basePath,
	}

	switch field.Type {
	case core.FieldTypeBoolean:
		checked, _ := value.(bool)
		data["Checked"] = checked

	case core.FieldTypeEnum:
		current := stringOrEmpty(value)
		options := make([]fieldOptionData, 0, len(field.Choices))
		for _, choice := range field.Choices {
			s := fmt.Sprint(choice)
			options = append(options, fieldOptionData{Value: s, Label: s, Selected: s == current})
		}
		data["Options"] = options

	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		autocomplete := relation != nil && relation.Autocomplete
		data["Autocomplete"] = autocomplete
		if autocomplete {
			selectedPK := ""
			if relation.SelectedPK != nil {
				selectedPK = fmt.Sprint(relation.SelectedPK)
			}
			data["ComboboxSelectedPK"] = selectedPK
			data["ComboboxSelectedLabel"] = relation.SelectedLabel
			data["ComboboxResultsID"] = "combobox-results-" + field.Name
			data["LookupTarget"] = relation.LookupTarget
		} else {
			options := make([]fieldOptionData, 0)
			if relation != nil {
				selectedPK := fmt.Sprint(relation.SelectedPK)
				for _, opt := range relation.Options {
					pk := fmt.Sprint(opt.PK)
					options = append(options, fieldOptionData{Value: pk, Label: opt.Label, Selected: pk == selectedPK})
				}
			}
			data["Options"] = options
		}

	case core.FieldTypeManyToMany:
		options := make([]fieldOptionData, 0)
		if relation != nil {
			selected := make(map[string]bool, len(relation.SelectedPKs))
			for _, spk := range relation.SelectedPKs {
				selected[fmt.Sprint(spk)] = true
			}
			for _, opt := range relation.Options {
				pk := fmt.Sprint(opt.PK)
				options = append(options, fieldOptionData{Value: pk, Label: opt.Label, Selected: selected[pk]})
			}
		}
		data["Options"] = options
	}

	return r.uiHTML("ui/field", data)
}

func stringOrEmpty(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprint(value)
}
