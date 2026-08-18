package core

import (
	"context"
	"errors"
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

// ModelAdmin is the central per-resource abstraction.
// Applications normally satisfy it by embedding BaseModelAdmin and
// shadowing whichever methods they need, rather than implementing every
// method from scratch.
type ModelAdmin interface {
	Slug() string
	VerboseName() string
	// Category groups this ModelAdmin (and any AdminPages sharing the
	// same category) into one sidebar accordion section, in
	// first-registration-appearance order. "" keeps today's flat
	// top-level nav link.
	Category() string
	// Icon names the sidebar-nav icon (see fiber/icons.go's iconPaths)
	// shown next to this ModelAdmin's own link, whether it renders flat
	// or nested inside a category's accordion. Defaults to "collection".
	Icon() string

	Fields() map[string]Field
	Field(name string) (Field, bool)
	ListDisplay() []string
	FormFields() []string
	SearchFields() []string
	DetailFields() []string
	Filters() []Filter
	Actions() []Action
	// Inlines declares child ModelAdmins whose records point back at
	// this one, managed/displayed inline on this ModelAdmin's
	// create/detail/edit pages. See core/inline.go and docs/inlines.md.
	Inlines() []Inline
	AutocompleteFields() []string
	GetPK(obj any) any
	TemplateOverride(view string) string

	CanView() bool
	CanCreate() bool
	CanUpdate() bool
	CanDelete() bool
	CanExport() bool

	ListDisplayValues(obj any) map[string]any
	Validate(data map[string]any) map[string][]string

	GetQueryset(ctx context.Context) (any, error)
	GetObject(ctx context.Context, pk any) (any, error)
	Create(ctx context.Context, data map[string]any) (any, error)
	Update(ctx context.Context, obj any, data map[string]any) (any, error)
	Delete(ctx context.Context, obj any) error
}

// BaseModelAdmin provides default implementations of ModelAdmin.
// Embed it in a resource-specific struct, set its declarative fields,
// and shadow the CRUD methods that need real data access:
//
//	type UserAdmin struct {
//	    core.BaseModelAdmin
//	}
//
//	func NewUserAdmin() UserAdmin {
//	    return UserAdmin{core.BaseModelAdmin{
//	        ModelName:     "User",
//	        DisplayFields: []string{"ID", "Email", "IsActive"},
//	    }}
//	}
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
	NavIcon          string
	DisplayFields    []string
	FormFieldNames   []string
	SearchFieldNames []string
	DetailFieldNames []string // nil means "derive from DisplayFields ++ FormFieldNames"
	DeclaredFields   []Field
	DeclaredFilters  []Filter
	DeclaredActions  []Action
	DeclaredInlines  []Inline
	// Relation (foreignkey/onetoone) form fields that render as a
	// lookup-driven search box instead of a <select>
	// populated from the target's full queryset -- for relations too
	// large, or too principal-sensitive, to dump wholesale into a page.
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

// TemplateOverride returns the explicit template path set for the
// given view ("list"/"detail"/"form"/"delete"), or "" if none is set
// -- mirrors the Python adapter's `{view}_template` ModelAdmin
// attributes. See docs/templates.md for the full override resolution
// order.
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

func (b BaseModelAdmin) ListDisplay() []string  { return b.DisplayFields }
func (b BaseModelAdmin) FormFields() []string   { return b.FormFieldNames }
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

func (b BaseModelAdmin) Filters() []Filter            { return b.DeclaredFilters }
func (b BaseModelAdmin) Actions() []Action            { return b.DeclaredActions }
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
func (b BaseModelAdmin) CanExport() bool { return !b.DisableExport }

func (b BaseModelAdmin) ListDisplayValues(obj any) map[string]any {
	fields := b.Fields()
	values := make(map[string]any, len(b.DisplayFields))
	for _, name := range b.DisplayFields {
		values[name] = fields[name].GetValue(obj)
	}
	return values
}

func (b BaseModelAdmin) Validate(data map[string]any) map[string][]string {
	fields := b.Fields()
	errs := make(map[string][]string)
	for _, name := range b.FormFieldNames {
		if fieldErrs := fields[name].Validate(data[name]); len(fieldErrs) > 0 {
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
