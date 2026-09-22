package fiber

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

// mustUI resolves a ui.go class string or panics. Called only from the
// package-level vars below, so an unknown name fails at program start
// rather than mid-request.
func mustUI(component string, modifiers ...string) string {
	classes, err := uiClasses(component, modifiers...)
	if err != nil {
		panic(err)
	}
	return classes
}

// The class strings this file emits directly, for the two renderers that
// build HTML by hand. formInputHTML does not use them: it delegates to the
// ui/field partial.
var (
	classInputCompact = mustUI("input", "size-sm")
	classSelectSmall  = mustUI("select", "size-sm")
	classSelectCell   = mustUI("select", "cell")
	classSelectMulti  = mustUI("select", "cell-multi")
	classScrollAreaY  = mustUI("scroll-area", "y")
	classCheckbox     = mustUI("checkbox")
	classTextError    = mustUI("text", "error")

	classPlaceholder = mustUI("text", "placeholder")
	classLink        = mustUI("text", "link")
)

// isBlank reports whether a value is an empty string or the zero
// time.Time. A zero int or a false bool are real values, and showing a
// dash for them would be a lie; the zero time (0001-01-01) is how a
// non-pointer time.Time field says "not set", never a real date.
func isBlank(value any) bool {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case time.Time:
		return v.IsZero()
	}
	return false
}

// fieldValueHTML renders a field's read-only value for list and detail,
// including permission-aware relation links. Built in Go rather than in
// a template so every dynamic string goes through html.EscapeString
// explicitly: template.HTML bypasses html/template's auto-escaping, so
// this is the one place that has to get escaping right by hand.
func (r *Renderer) fieldValueHTML(relationPermissions map[string]bool, field core.Field, value any, emptyValue string) template.HTML {
	if emptyValue == "" {
		emptyValue = core.DefaultEmptyValue
	}
	dash := template.HTML(`<span class="` + classPlaceholder + `">` + html.EscapeString(emptyValue) + `</span>`)
	// Blank counts as empty, not just nil: a column of empty cells and a
	// column of dashes say different things to a reader, and "" is by
	// far the more common way a value goes missing in practice.
	if core.IsNil(value) || isBlank(value) {
		return dash
	}
	switch field.Type {
	case core.FieldTypeBoolean:
		if b, _ := value.(bool); b {
			// The reference design system has no "success" token to defer
			// to, so this picks an emerald pair that clears contrast
			// against bg-card in both themes.
			return boolIconHTML("check", "text-emerald-600 dark:text-emerald-400", r.t("Yes"))
		}
		return boolIconHTML("close", classPlaceholder, r.t("No"))
	case core.FieldTypePassword:
		return template.HTML(`<span class="` + classPlaceholder + `">&bull;&bull;&bull;&bull;&bull;&bull;&bull;&bull;</span>`)
	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		return relatedLinkHTML(r.admin, r.basePath, relationPermissions, field.Relation, value)
	case core.FieldTypeManyToMany:
		items, _ := value.([]any)
		if len(items) == 0 {
			return dash
		}
		parts := make([]string, len(items))
		for i, item := range items {
			parts[i] = string(relatedLinkHTML(r.admin, r.basePath, relationPermissions, field.Relation, item))
		}
		return template.HTML(strings.Join(parts, ", "))
	case core.FieldTypeDate:
		if iso, ok := isoDate(value); ok {
			return template.HTML(`<time datetime="` + iso + `" data-format="date">` + iso + `</time>`)
		}
	case core.FieldTypeDateTime:
		if iso, ok := isoDateTime(value); ok {
			return template.HTML(`<time datetime="` + iso + `" data-format="datetime">` + iso + `</time>`)
		}
	case core.FieldTypeDecimal:
		raw := html.EscapeString(decimalText(value))
		return template.HTML(`<span data-format="decimal" data-value="` + raw + `">` + raw + `</span>`)
	}
	return template.HTML(html.EscapeString(fmt.Sprint(value)))
}

// decimalText renders a decimal value as fixed-point text. fmt.Sprint on
// a float64 switches to scientific notation past a certain magnitude
// (1e+21), which the browser-side formatter's fraction-digit count
// (theme.html's polyadminFormat, counting digits after ".") can't read;
// strconv.FormatFloat with a negative precision keeps the shortest exact
// fixed-point form instead, matching what the data-value attribute and
// its visible fallback text both need to agree on.
func decimalText(value any) string {
	if f, ok := value.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}

var (
	isoDatePattern     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	isoDateTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:\d{2})?$`)
)

// isoDate returns a date as YYYY-MM-DD, the only form the browser-side
// formatter reads for a date-only value.
func isoDate(value any) (string, bool) {
	switch v := value.(type) {
	case time.Time:
		return v.Format("2006-01-02"), true
	case string:
		return v, isoDatePattern.MatchString(v)
	}
	return "", false
}

// isoDateTime returns RFC 3339. A time.Time always knows its zone, so it
// always carries an offset; a string without one is naive and is shown as
// wall-clock time, unconverted.
func isoDateTime(value any) (string, bool) {
	switch v := value.(type) {
	case time.Time:
		return v.Format(time.RFC3339), true
	case string:
		return v, isoDateTimePattern.MatchString(v)
	}
	return "", false
}

// boolIconHTML renders a boolean as a check or a cross rather than
// "Yes"/"No": a glyph reads as a shape at a glance where two similar-
// length words have to be read. The word stays as an sr-only label, so
// nothing depends on the icon alone. Exports stringify through
// core/exporter.go and are untouched. `class` is a package constant;
// `label` is translated, and a catalog is not markup, so it is escaped.
func boolIconHTML(icon, class, label string) template.HTML {
	return template.HTML(`<span class="inline-flex items-center ` + class + `">` +
		string(iconHTML(icon, "size-4")) +
		`<span class="sr-only">` + html.EscapeString(label) + `</span></span>`)
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

// formInputHTML renders a field's form input as the reference design
// system's Form/FormItem unit -- label, control, description, error -- resolving every per-type
// value here in Go and handing the result to the ui/field partial to
// dispatch and print. Unlike fieldValueHTML, escaping is html/template's
// own, since every value reaches the template as a plain field on the
// data map rather than a hand-built HTML string.
//
// The readOnly argument renders the field as its value instead of a
// control, which is distinct from core.Field.ReadOnly (a native
// `readonly` input, which still posts): it removes the control
// entirely, and pairs with parseFormData refusing the name.
//
// A method on Renderer only because it renders through r.uiHTML.
func (r *Renderer) formInputHTML(basePath string, field core.Field, value any, errs []string, relation *relationFieldOptions, readOnly bool) (template.HTML, error) {
	data := map[string]any{
		"ReadOnlyField": readOnly,
		"Name":          field.Name,
		"Label":         field.Label,
		"Required":      field.Required,
		"ReadOnly":      field.ReadOnly,
		"Disabled":      field.Disabled,
		"Placeholder":   field.Placeholder,
		"HelpText":      field.HelpText,
		"Type":          string(field.Type),
		"InputType":     inputTypeFor(field.Type),
		"StringValue":   inputValue(field.Type, value),
		"Errors":        errs,
		"BasePath":      basePath,
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

// inputValue is a field's value as its form control's value attribute.
// A date or datetime-local input discards anything but its own ISO form
// (and fmt.Sprint of a time.Time is not that), so a time.Time is written
// as YYYY-MM-DD or YYYY-MM-DDTHH:MM in its own zone -- the wall-clock
// time, as the input shows it -- and the zero time as empty.
func inputValue(fieldType core.FieldType, value any) string {
	if v, ok := value.(time.Time); ok {
		switch {
		case v.IsZero():
			return ""
		case fieldType == core.FieldTypeDate:
			return v.Format("2006-01-02")
		case fieldType == core.FieldTypeDateTime:
			return v.Format("2006-01-02T15:04")
		}
	}
	return stringOrEmpty(value)
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
