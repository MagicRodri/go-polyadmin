package fiber

import (
	"fmt"
	"html"
	"html/template"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
)

// fieldValueHTML renders a field's read-only value (list/detail),
// including permission-aware relation links. Built in
// Go rather than inside a template so every dynamic bit of text goes
// through html.EscapeString explicitly -- template.HTML bypasses
// html/template's auto-escaping, so this function is the one place
// that has to get escaping right by hand.
func fieldValueHTML(admin *core.Admin, basePath string, relationPermissions map[string]bool, field core.Field, value any) template.HTML {
	if core.IsNil(value) {
		return `<span class="text-neutral-400">&mdash;</span>`
	}
	switch field.Type {
	case core.FieldTypeBoolean:
		if b, _ := value.(bool); b {
			return `<span class="text-green-700">Yes</span>`
		}
		return `<span class="text-neutral-400">No</span>`
	case core.FieldTypePassword:
		return `<span class="text-neutral-400">&bull;&bull;&bull;&bull;&bull;&bull;&bull;&bull;</span>`
	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		return relatedLinkHTML(admin, basePath, relationPermissions, field.Relation, value)
	case core.FieldTypeManyToMany:
		items, _ := value.([]any)
		if len(items) == 0 {
			return `<span class="text-neutral-400">&mdash;</span>`
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
	return template.HTML(fmt.Sprintf(`<a class="text-blue-600 hover:underline" href="%s">%s</a>`, href, label))
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
	// combobox branch of formInputHTML in place of the plain
	// <select> above -- see core.BaseModelAdmin.AutocompleteFieldNames.
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

// pinesFieldClasses matches PinesUI's text-input/textarea/select
// conventions (devdojo.com/pines/docs/text-input): neutral-* border
// and focus ring, no separate JS component. PinesUI's own <select> is
// a bespoke Alpine widget that doesn't map onto a plain form POST, so
// relation/enum selects here stay native <select> elements styled to
// match, same tradeoff as the Python/Jinja side.
const pinesFieldClasses = "mt-1 flex h-10 w-full rounded-md border border-neutral-300 bg-white px-3 py-2 text-sm placeholder:text-neutral-500 focus:border-neutral-400 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50"

// formInputHTML renders a field's form input, the Go
// counterpart to fieldValueHTML -- same care around manual escaping
// applies here.
func formInputHTML(basePath string, field core.Field, value any, errs []string, relation *relationFieldOptions) template.HTML {
	name := html.EscapeString(field.Name)
	label := html.EscapeString(field.Label)

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="mb-4"><label for="field-%s" class="block text-sm font-medium text-neutral-700">%s`, name, label)
	if field.Required {
		b.WriteString(" *")
	}
	b.WriteString(`</label>`)

	switch field.Type {
	case core.FieldTypeText:
		fmt.Fprintf(&b, `<textarea id="field-%s" name="%s" autocomplete="off" %s %s %s class="%s h-auto min-h-[80px]">%s</textarea>`,
			name, name, attr(field.Required, "required"), attr(field.ReadOnly, "readonly"), attr(field.Disabled, "disabled"),
			pinesFieldClasses, html.EscapeString(stringOrEmpty(value)))

	case core.FieldTypeBoolean:
		checked, _ := value.(bool)
		fmt.Fprintf(&b, `<input type="checkbox" id="field-%s" name="%s" value="true" autocomplete="off" %s %s class="mt-1 h-4 w-4 rounded border-neutral-300 bg-neutral-100 text-neutral-900 focus:outline-none">`,
			name, name, attr(checked, "checked"), attr(field.Disabled, "disabled"))

	case core.FieldTypeEnum:
		fmt.Fprintf(&b, `<select id="field-%s" name="%s" autocomplete="off" %s class="%s">`,
			name, name, attr(field.Disabled, "disabled"), pinesFieldClasses)
		for _, choice := range field.Choices {
			choiceStr := fmt.Sprint(choice)
			selected := choiceStr == stringOrEmpty(value)
			fmt.Fprintf(&b, `<option value="%s" %s>%s</option>`, html.EscapeString(choiceStr), attr(selected, "selected"), html.EscapeString(choiceStr))
		}
		b.WriteString(`</select>`)

	case core.FieldTypeForeignKey, core.FieldTypeOneToOne:
		if relation != nil && relation.Autocomplete {
			b.WriteString(autocompleteComboboxHTML(basePath, name, relation, field.Disabled))
			break
		}
		fmt.Fprintf(&b, `<select id="field-%s" name="%s" autocomplete="off" %s class="%s"><option value="">&mdash;</option>`,
			name, name, attr(field.Disabled, "disabled"), pinesFieldClasses)
		if relation != nil {
			for _, opt := range relation.Options {
				selected := fmt.Sprint(opt.PK) == fmt.Sprint(relation.SelectedPK)
				fmt.Fprintf(&b, `<option value="%s" %s>%s</option>`, html.EscapeString(fmt.Sprint(opt.PK)), attr(selected, "selected"), html.EscapeString(opt.Label))
			}
		}
		b.WriteString(`</select>`)

	case core.FieldTypeManyToMany:
		size := 1
		if relation != nil && len(relation.Options) > size {
			size = len(relation.Options)
		}
		fmt.Fprintf(&b, `<select multiple id="field-%s" name="%s" autocomplete="off" %s size="%d" class="%s h-auto">`,
			name, name, attr(field.Disabled, "disabled"), size, pinesFieldClasses)
		if relation != nil {
			for _, opt := range relation.Options {
				selected := false
				for _, spk := range relation.SelectedPKs {
					if fmt.Sprint(spk) == fmt.Sprint(opt.PK) {
						selected = true
						break
					}
				}
				fmt.Fprintf(&b, `<option value="%s" %s>%s</option>`, html.EscapeString(fmt.Sprint(opt.PK)), attr(selected, "selected"), html.EscapeString(opt.Label))
			}
		}
		b.WriteString(`</select>`)

	default:
		fmt.Fprintf(&b, `<input type="%s" id="field-%s" name="%s" value="%s" placeholder="%s" autocomplete="off" %s %s %s class="%s">`,
			inputTypeFor(field.Type), name, name, html.EscapeString(stringOrEmpty(value)), html.EscapeString(field.Placeholder),
			attr(field.Required, "required"), attr(field.ReadOnly, "readonly"), attr(field.Disabled, "disabled"), pinesFieldClasses)
	}

	if field.HelpText != "" {
		fmt.Fprintf(&b, `<p class="mt-1 text-xs text-neutral-500">%s</p>`, html.EscapeString(field.HelpText))
	}
	for _, e := range errs {
		fmt.Fprintf(&b, `<p class="mt-1 text-xs text-red-600">%s</p>`, html.EscapeString(e))
	}
	b.WriteString(`</div>`)
	return template.HTML(b.String())
}

// autocompleteComboboxHTML renders a lookup-driven searchable command
// , adapted from PinesUI's Command component
// (devdojo.com/pines/docs/command) -- Pines only documents a
// full-screen modal palette over a client-side item array; this keeps
// its visual language (bordered panel, search icon, item hover/active
// highlighting, arrow-key navigation) but swaps the client-side filter
// for the /lookup route's server-side search, since the whole point of
// AutocompleteFieldNames is never loading the target's full dataset
// into the page. Mirrors python/admin/templates/admin/components/form.html's
// same branch -- see its comment for why pk/label never flow through
// an interpolated JS expression here either, and for why selectItem
// writes a new pk straight to $refs.hiddenInput.value imperatively
// rather than through a reactive `:value` binding (x-bind directives
// run before x-init, so a `:value` tied to x-data state would blank
// the field's server-rendered value on every page load, before x-init
// ever got a chance to read it back out).
func autocompleteComboboxHTML(basePath, name string, relation *relationFieldOptions, disabled bool) string {
	selectedPK := ""
	if relation.SelectedPK != nil {
		selectedPK = html.EscapeString(fmt.Sprint(relation.SelectedPK))
	}
	selectedLabel := html.EscapeString(relation.SelectedLabel)
	lookupTarget := html.EscapeString(relation.LookupTarget)
	resultsID := "combobox-results-" + name

	return fmt.Sprintf(`<div class="relative"
     x-data="{
       open: false, query: '', activeEl: null,
       selectItem(pk, label) { this.$refs.hiddenInput.value = pk; this.query = label; this.open = false; this.activeEl = null; },
       moveActive(delta) {
         const items = Array.from(this.$refs.results.children).filter(el => el.dataset.pk !== undefined);
         if (!items.length) return;
         let idx = items.indexOf(this.activeEl);
         idx = (idx + delta + items.length) %% items.length;
         items.forEach(el => el.classList.remove('bg-neutral-100'));
         this.activeEl = items[idx];
         this.activeEl.classList.add('bg-neutral-100');
         this.activeEl.scrollIntoView({ block: 'nearest' });
       },
       selectActive() { if (this.activeEl) this.activeEl.click(); }
     }"
     x-init="query = $refs.textInput.value;"
     @click.outside="open = false">
  <input x-ref="hiddenInput" type="hidden" name="%s" value="%s">
  <div class="flex items-center gap-2 rounded-md border border-neutral-300 bg-white px-3 focus-within:border-neutral-400 focus-within:outline-none">
    %s
    <input x-ref="textInput" type="text" autocomplete="off" placeholder="Search&hellip;"
           value="%s" x-model="query" @focus="open = true"
           @keydown.escape="open = false" @keydown.down.prevent="open = true; moveActive(1)"
           @keydown.up.prevent="moveActive(-1)" @keydown.enter.prevent="selectActive()"
           %s
           hx-get="%s/%s/lookup" hx-trigger="keyup changed delay:300ms, focus"
           hx-target="#%s" hx-swap="innerHTML"
           class="h-10 w-full border-0 bg-transparent p-0 text-sm outline-none focus:ring-0 placeholder:text-neutral-400">
  </div>
  <div x-ref="results" id="%s"
       class="absolute z-10 mt-1 max-h-[280px] w-full overflow-y-auto rounded-md border border-neutral-200 bg-white py-1 text-sm shadow-md"
       x-show="open" x-cloak></div>
</div>`,
		name, selectedPK,
		iconHTML("search", "w-4 h-4 shrink-0 text-neutral-400"),
		selectedLabel, attr(disabled, "disabled"), basePath, lookupTarget, resultsID, resultsID)
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
