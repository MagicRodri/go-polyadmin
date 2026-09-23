package core

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// ErrNotImplemented is returned by the default CRUD data-access hooks.
// Concrete ModelAdmins are expected to shadow GetQueryset / GetObject /
// Create / Update / Delete.
var ErrNotImplemented = errors.New("polyadmin: not implemented")

// defaultIcon is the sidebar-nav icon shown when a ModelAdmin/AdminPage
// doesn't set its own -- matches every resource link's icon before
// per-item icons existed, so it's the unsurprising default.
const defaultIcon = "collection"

// ModelAdmin is the central per-resource abstraction. Applications
// normally satisfy it by embedding BaseModelAdmin and shadowing only the
// methods they need.
type ModelAdmin interface {
	Slug() string
	VerboseName() string
	// Category groups this ModelAdmin, and any AdminPages sharing the
	// category, into one sidebar section. "" keeps a flat top-level link.
	Category() string
	// Icon names the sidebar-nav icon (see fiber/icons.go's iconPaths)
	// shown next to this ModelAdmin's own link, whether it renders flat
	// or nested inside a category's accordion. Defaults to "collection".
	Icon() string
	// FaviconURL is the browser-tab icon shown while viewing this
	// ModelAdmin's pages. Empty falls back to Admin.SiteFaviconURL.
	FaviconURL() string

	Fields() map[string]Field
	Field(name string) (Field, bool)
	ListDisplay() []string
	FormFields() []string
	Fieldsets() []Fieldset
	ReadOnlyFields(obj any) []string
	DefaultOrdering() string
	PageSize() int
	EmptyValue() string
	IsReadOnly(name string, obj any) bool
	SearchFields() []string
	DetailFields() []string
	Filters() []Filter
	Actions() []Action
	// DetailActions names the actions the detail page offers, in order;
	// nil means "use each action's Where". See ActionsForDetail.
	DetailActions() []string
	// Inlines declares child ModelAdmins whose records point back at
	// this one, managed/displayed inline on this ModelAdmin's
	// create/detail/edit pages. See core/inline.go and docs/inlines.md.
	Inlines() []Inline
	AutocompleteFields() []string
	// The small-parity options -- see docs/lists.md and
	// docs/model-admin.md. SortableFields and LinkFields distinguish nil
	// ("unset") from empty ("none"), so both return the declared slice
	// rather than a resolved one; IsSortable and LinksToRecord apply the
	// defaults.
	SortableFields() []string
	LinkFields() []string
	Prepopulated() map[string][]string
	AllowsSaveAs() bool
	PreservesFilters() bool
	GetPK(obj any) any
	TemplateOverride(view string) string

	CanView() bool
	CanCreate() bool
	CanUpdate() bool
	CanDelete() bool
	CanExport() bool
	// Reorderable shows a drag handle on the list view. Dragging never
	// persists: it reorders the <tr> elements on the page and reverts on the
	// next render. It is for triaging a list by hand without the framework
	// taking a position on how that order would be stored.
	Reorderable() bool

	ListDisplayValues(obj any) map[string]any
	Validate(ctx context.Context, data map[string]any) map[string][]string

	GetQueryset(ctx context.Context) (any, error)
	GetObject(ctx context.Context, pk any) (any, error)
	Create(ctx context.Context, data map[string]any) (any, error)
	Update(ctx context.Context, obj any, data map[string]any) (any, error)
	Delete(ctx context.Context, obj any) error
}

// Fieldset is one titled group of form fields. A Title of "" renders
// the group with no header, which is how the
// undeclared default renders as a plain flat form. Collapsed only seeds
// the initial state; the group can always be opened.
type Fieldset struct {
	Title       string
	Description string
	Collapsed   bool
	Fields      []string
}

// BaseModelAdmin provides default implementations of ModelAdmin. Embed
// it in a resource-specific struct, set its declarative fields, and
// shadow the CRUD methods that need real data access:
//
//	type UserAdmin struct{ core.BaseModelAdmin }
//
//	func (a UserAdmin) GetQueryset(ctx context.Context) (any, error) { ... }
type BaseModelAdmin struct {
	ModelName    string
	SlugOverride string
	// NavCategory backs Category() -- named separately since a Go
	// struct can't have a field and a method share the name "Category".
	NavCategory string
	// NavIcon backs Icon() -- named separately for the same reason as
	// NavCategory/Category(). Empty means defaultIcon.
	NavIcon string
	// NavFaviconURL backs FaviconURL() -- named separately for the same
	// reason as NavCategory/Category(). Empty means "no override";
	// FaviconURL() itself falls back to Admin.SiteFaviconURL.
	NavFaviconURL    string
	DisplayFields    []string
	FormFieldNames   []string
	SearchFieldNames []string
	DetailFieldNames []string // nil means "derive from DisplayFields ++ FormFieldNames"
	DeclaredFields   []Field
	DeclaredFilters  []Filter
	DeclaredActions  []Action
	// DeclaredDetailActions names the actions the detail page offers, in
	// order. nil means every action whose Where includes the detail page; a
	// non-nil slice overrides Where, and an empty one offers none.
	DeclaredDetailActions []string
	DeclaredInlines       []Inline
	// DeclaredFieldsets, when set, defines both the grouping and the field
	// list: FormFields() reports it flattened, so the form and the handler
	// agree. FormFieldNames is then unused.
	DeclaredFieldsets []Fieldset
	// ReadOnlyFieldNames render as values, not inputs, and are refused if
	// posted anyway. Override ReadOnlyFields to vary by object, which is how
	// "editable on create, frozen afterwards" is expressed.
	ReadOnlyFieldNames []string
	// OrderingDefault is the sort applied when a request names none, in the
	// ?sort= syntax ("-field" for descending). Without one, rows arrive in
	// whatever order the data source returned, which for a map-backed store
	// is not stable between requests.
	OrderingDefault string
	// PageSizeDefault is how many rows a list page holds; zero means
	// DefaultPageSize.
	PageSizeDefault int
	// EmptyValueDisplay is what a read-only view shows in place of a
	// value that is nil or blank. Empty means DefaultEmptyValue.
	EmptyValueDisplay string
	// DisableDeleteSelected removes the built-in bulk delete. Disable*, so
	// the zero value keeps it.
	DisableDeleteSelected bool
	// Relation fields that render as a lookup-driven search box rather than a
	// <select> over the target's full queryset -- for relations too large, or
	// too principal-sensitive, to dump into a page.
	AutocompleteFieldNames []string
	// PK returns the primary key used to build this object's URL,
	// defaulting to reading an exported "ID" field via reflection.
	PK func(obj any) any

	// Explicit per-view template overrides -- see TemplateOverride and
	// docs/templates.md. Empty means "no override for this view", the
	// zero value's natural meaning.
	ListTemplate   string
	DetailTemplate string
	FormTemplate   string
	DeleteTemplate string

	// Disable* default to false (enabled), matching Go's zero value.
	DisableView   bool
	DisableCreate bool
	DisableUpdate bool
	DisableDelete bool
	DisableExport bool

	// EnableReordering defaults to false (opt-in, unlike Disable*
	// above) -- see the doc comment on Reorderable().
	EnableReordering bool

	// SortableFieldNames restricts which list columns offer a sort. nil
	// leaves every column sortable; an empty (non-nil) slice makes none
	// of them sortable. The restriction also holds for a hand-typed
	// ?sort=, but not for OrderingDefault, which is the admin's own
	// choice rather than user input.
	SortableFieldNames []string
	// LinkFieldNames names the list cells that link to the record. nil
	// links the first column; an empty (non-nil) slice links none,
	// leaving the row menu as the way in.
	LinkFieldNames []string
	// PrepopulatedFields fills a field from others as they are typed:
	// {"Slug": {"Title"}} slugifies Title into Slug. Client-side and on
	// the create form only, so an existing record's slug is never
	// rewritten under it.
	PrepopulatedFields map[string][]string
	// PrepopulatedUnicode keeps a prepopulated field's letters as they
	// are instead of transliterating them to ASCII -- see Slugify.
	PrepopulatedUnicode []string
	// AllowSaveAs adds "Save as new" to the edit form, which saves the
	// submitted values as a new record and leaves the original alone.
	// Opt-in.
	AllowSaveAs bool
	// DisablePreserveFilters stops the list handing its search, filters,
	// sort and page to the pages reached from it, so they no longer lead
	// back into the list as it was left. Disable*, because preserving
	// them is the behaviour worth defaulting to.
	DisablePreserveFilters bool
}

func (b BaseModelAdmin) Slug() string {
	if b.SlugOverride != "" {
		return b.SlugOverride
	}
	return strings.ToLower(b.ModelName) + "s"
}

func (b BaseModelAdmin) VerboseName() string {
	return b.ModelName
}

func (b BaseModelAdmin) Category() string {
	return b.NavCategory
}

func (b BaseModelAdmin) Icon() string {
	if b.NavIcon != "" {
		return b.NavIcon
	}
	return defaultIcon
}

func (b BaseModelAdmin) FaviconURL() string {
	return b.NavFaviconURL
}

// TemplateOverride returns the explicit template path set for the given
// view ("list"/"detail"/"form"/"delete"), or "". See docs/templates.md for
// the full resolution order.
func (b BaseModelAdmin) TemplateOverride(view string) string {
	switch view {
	case "list":
		return b.ListTemplate
	case "detail":
		return b.DetailTemplate
	case "form":
		return b.FormTemplate
	case "delete":
		return b.DeleteTemplate
	default:
		return ""
	}
}

func (b BaseModelAdmin) Fields() map[string]Field {
	fields := make(map[string]Field, len(b.DeclaredFields))
	for _, f := range b.DeclaredFields {
		fields[f.Name] = f
	}
	for _, name := range b.impliedFieldNames() {
		if _, ok := fields[name]; !ok {
			fields[name] = NewField(name, FieldTypeUnspecified)
		}
	}
	return fields
}

func (b BaseModelAdmin) impliedFieldNames() []string {
	names := make([]string, 0, len(b.DisplayFields)+len(b.FormFieldNames)+len(b.SearchFieldNames))
	names = append(names, b.DisplayFields...)
	names = append(names, b.FormFieldNames...)
	names = append(names, b.SearchFieldNames...)
	return names
}

func (b BaseModelAdmin) Field(name string) (Field, bool) {
	f, ok := b.Fields()[name]
	return f, ok
}

func (b BaseModelAdmin) ListDisplay() []string { return b.DisplayFields }
func (b BaseModelAdmin) FormFields() []string {
	if len(b.DeclaredFieldsets) == 0 {
		return b.FormFieldNames
	}
	var names []string
	for _, set := range b.DeclaredFieldsets {
		names = append(names, set.Fields...)
	}
	return names
}

// ReadOnlyFields returns the fields rendering as values rather than inputs
// for this object. obj is nil on the create form, so an override can tell
// creating from editing.
func (b BaseModelAdmin) ReadOnlyFields(obj any) []string { return b.ReadOnlyFieldNames }

// DefaultOrdering is the sort to use when the request names none.
func (b BaseModelAdmin) DefaultOrdering() string { return b.OrderingDefault }

// PageSize is the ModelAdmin's own page size, or 0 to accept the
// framework default. See PageSizeDefault.
func (b BaseModelAdmin) PageSize() int { return b.PageSizeDefault }

// EmptyValue is what stands in for a nil or blank value on the list and
// detail views. See EmptyValueDisplay.
func (b BaseModelAdmin) EmptyValue() string {
	if b.EmptyValueDisplay == "" {
		return DefaultEmptyValue
	}
	return b.EmptyValueDisplay
}

// IsReadOnly is the question every call site actually asks.
func (b BaseModelAdmin) IsReadOnly(name string, obj any) bool {
	return slices.Contains(b.ReadOnlyFields(obj), name)
}

// Fieldsets always returns at least one group: the form template renders
// groups unconditionally, so "none declared" means one unnamed group
// holding every form field, not zero groups holding nothing.
func (b BaseModelAdmin) Fieldsets() []Fieldset {
	if len(b.DeclaredFieldsets) == 0 {
		return []Fieldset{{Fields: b.FormFieldNames}}
	}
	return b.DeclaredFieldsets
}
func (b BaseModelAdmin) SearchFields() []string { return b.SearchFieldNames }

func (b BaseModelAdmin) DetailFields() []string {
	if b.DetailFieldNames != nil {
		return b.DetailFieldNames
	}
	seen := make(map[string]bool)
	var names []string
	for _, name := range append(append([]string{}, b.DisplayFields...), b.FormFieldNames...) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

func (b BaseModelAdmin) Filters() []Filter { return b.DeclaredFilters }

// Actions are the declared ones plus the built-in bulk delete every admin
// that can delete gets for free. Declaring one named DeleteSelectedName
// replaces it rather than duplicating it.
func (b BaseModelAdmin) Actions() []Action {
	if b.DisableDeleteSelected || !b.CanDelete() {
		return b.DeclaredActions
	}
	for _, action := range b.DeclaredActions {
		if action.Name == DeleteSelectedName {
			return b.DeclaredActions
		}
	}
	// Appended, not prepended: an application's own actions are the ones
	// it went to the trouble of writing, and the destructive one should
	// not be the first thing in the listbox.
	return append(append([]Action{}, b.DeclaredActions...), NewDeleteSelectedAction())
}
func (b BaseModelAdmin) DetailActions() []string      { return b.DeclaredDetailActions }
func (b BaseModelAdmin) Inlines() []Inline            { return b.DeclaredInlines }
func (b BaseModelAdmin) AutocompleteFields() []string { return b.AutocompleteFieldNames }

func (b BaseModelAdmin) GetPK(obj any) any {
	if b.PK != nil {
		return b.PK(obj)
	}
	return defaultPK(obj)
}

func (b BaseModelAdmin) CanView() bool   { return !b.DisableView }
func (b BaseModelAdmin) CanCreate() bool { return !b.DisableCreate }
func (b BaseModelAdmin) CanUpdate() bool { return !b.DisableUpdate }
func (b BaseModelAdmin) CanDelete() bool { return !b.DisableDelete }

func (b BaseModelAdmin) SortableFields() []string { return b.SortableFieldNames }

func (b BaseModelAdmin) LinkFields() []string { return b.LinkFieldNames }

func (b BaseModelAdmin) Prepopulated() map[string][]string { return b.PrepopulatedFields }

// UnicodeSlugFields are the prepopulated fields whose slugs keep their own
// letters instead of being transliterated -- see Slugify.
func (b BaseModelAdmin) UnicodeSlugFields() []string { return b.PrepopulatedUnicode }

func (b BaseModelAdmin) AllowsSaveAs() bool { return b.AllowSaveAs }

func (b BaseModelAdmin) PreservesFilters() bool { return !b.DisablePreserveFilters }
func (b BaseModelAdmin) CanExport() bool        { return !b.DisableExport }

func (b BaseModelAdmin) Reorderable() bool { return b.EnableReordering }

func (b BaseModelAdmin) ListDisplayValues(obj any) map[string]any {
	fields := b.Fields()
	values := make(map[string]any, len(b.DisplayFields))
	for _, name := range b.DisplayFields {
		values[name] = fields[name].GetValue(obj)
	}
	return values
}

func (b BaseModelAdmin) Validate(ctx context.Context, data map[string]any) map[string][]string {
	fields := b.Fields()
	errs := make(map[string][]string)
	for _, name := range b.FormFieldNames {
		if fieldErrs := fields[name].Validate(ctx, data[name]); len(fieldErrs) > 0 {
			errs[name] = fieldErrs
		}
	}
	return errs
}

func (b BaseModelAdmin) GetQueryset(ctx context.Context) (any, error) {
	return nil, ErrNotImplemented
}

func (b BaseModelAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	return nil, ErrNotImplemented
}

func (b BaseModelAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	return nil, ErrNotImplemented
}

func (b BaseModelAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	return nil, ErrNotImplemented
}

func (b BaseModelAdmin) Delete(ctx context.Context, obj any) error {
	return ErrNotImplemented
}

// defaultPK reads a struct field or map key named "ID" via reflection.
func defaultPK(obj any) any {
	value, _ := structFieldOrMapValue(obj, "ID")
	return value
}
