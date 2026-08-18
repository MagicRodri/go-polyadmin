// Package core implements the language-agnostic admin concepts: Admin,
// ModelAdmin, Field, Relation, Filter, the list query pipeline,
// Authenticator/Authorizer, Dashboard/Widget, and Exporter. Rendering
// and routing are adapter concerns (see go/polyadmin/fiber).
package core

import (
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// FieldType tags a Field with its scalar kind. Relation
// field types (ForeignKey, OneToOne, ManyToMany) are added in Phase 6.
type FieldType string

const (
	FieldTypeUnspecified FieldType = "field"
	FieldTypeString      FieldType = "string"
	FieldTypeText        FieldType = "text"
	FieldTypeInteger     FieldType = "integer"
	FieldTypeDecimal     FieldType = "decimal"
	FieldTypeBoolean     FieldType = "boolean"
	FieldTypeDate        FieldType = "date"
	FieldTypeDateTime    FieldType = "datetime"
	FieldTypeEmail       FieldType = "email"
	FieldTypeURL         FieldType = "url"
	FieldTypeUUID        FieldType = "uuid"
	FieldTypeEnum        FieldType = "enum"
	FieldTypeJSON        FieldType = "json"
	FieldTypePassword    FieldType = "password"

	// Relation field types. Set Field.Relation alongside one
	// of these.
	FieldTypeForeignKey FieldType = "foreignkey"
	FieldTypeOneToOne   FieldType = "onetoone"
	FieldTypeManyToMany FieldType = "manytomany"
)

// Validator returns a non-nil error when value is invalid.
type Validator func(value any) error

// Field represents a model property plus its admin presentation; it is
// not merely an HTML input.
type Field struct {
	Name        string
	Label       string
	Type        FieldType
	Required    bool
	ReadOnly    bool
	Disabled    bool
	Default     any
	HasDefault  bool
	HelpText    string
	Placeholder string
	Validators  []Validator
	Choices     []any     // meaningful for FieldTypeEnum
	Relation    *Relation // set for FieldTypeForeignKey / OneToOne / ManyToMany

	// Get overrides value lookup used by GetValue. When nil, GetValue
	// looks up a struct field or map key equal to Name.
	Get func(obj any) any
}

// FieldOption configures a Field built with NewField.
type FieldOption func(*Field)

func WithLabel(label string) FieldOption {
	return func(f *Field) { f.Label = label }
}

func WithRequired() FieldOption {
	return func(f *Field) { f.Required = true }
}

func WithReadOnly() FieldOption {
	return func(f *Field) { f.ReadOnly = true }
}

func WithDisabled() FieldOption {
	return func(f *Field) { f.Disabled = true }
}

func WithDefault(value any) FieldOption {
	return func(f *Field) {
		f.Default = value
		f.HasDefault = true
	}
}

func WithHelpText(text string) FieldOption {
	return func(f *Field) { f.HelpText = text }
}

func WithPlaceholder(text string) FieldOption {
	return func(f *Field) { f.Placeholder = text }
}

func WithValidators(validators ...Validator) FieldOption {
	return func(f *Field) { f.Validators = append(f.Validators, validators...) }
}

func WithChoices(choices ...any) FieldOption {
	return func(f *Field) { f.Choices = choices }
}

func WithGetter(get func(obj any) any) FieldOption {
	return func(f *Field) { f.Get = get }
}

func WithRelation(relation Relation) FieldOption {
	return func(f *Field) { f.Relation = &relation }
}

// NewField builds a Field of the given type with a default label
// derived from name (e.g. "created_at" -> "Created At").
func NewField(name string, fieldType FieldType, opts ...FieldOption) Field {
	f := Field{Name: name, Label: defaultLabel(name), Type: fieldType}
	for _, opt := range opts {
		opt(&f)
	}
	return f
}

// defaultLabel turns a field name into a human-readable label, splitting
// on '_'/'-' (e.g. "created_at") and on lower-to-upper transitions (e.g.
// "IsActive", so idiomatic Go/PascalCase field names read naturally too).
func defaultLabel(name string) string {
	var words []string
	var current []rune
	runes := []rune(name)
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	for i, r := range runes {
		switch {
		case r == '_' || r == '-':
			flush()
		case i > 0 && unicode.IsUpper(r) && !unicode.IsUpper(runes[i-1]):
			flush()
			current = append(current, r)
		default:
			current = append(current, r)
		}
	}
	flush()
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}

// GetValue reads this field's value off a model instance: a struct
// field or map key equal to Name, unless Get is set. For a relation
// field (Relation != nil), the value is the related object(s)
// themselves, resolved via Relation.GetValue -- ManyToMany defaults to
// an empty slice rather than nil when unset.
func (f Field) GetValue(obj any) any {
	var value any
	found := true
	switch {
	case f.Get != nil:
		value = f.Get(obj)
	case f.Relation != nil:
		value = f.Relation.GetValue(obj)
	default:
		value, found = structFieldOrMapValue(obj, f.Name)
	}
	if !found || isNilAny(value) {
		if f.Type == FieldTypeManyToMany {
			return []any{}
		}
		return f.defaultValue()
	}
	return value
}

// IsNil reports whether value is nil, including a typed nil stored in
// an `any` (e.g. a nil *Organization) -- value == nil alone misses
// that case since the interface itself is non-nil. Exported for
// adapters (e.g. go/polyadmin/fiber) that need the same check when
// rendering a field's value.
func IsNil(value any) bool { return isNilAny(value) }

func isNilAny(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func (f Field) defaultValue() any {
	if f.HasDefault {
		return f.Default
	}
	return nil
}

// ParseFormValue coerces a raw submitted form string into this
// field's Go value. An empty string becomes nil for every field type
// so Validate's required check (which treats nil and "" alike) still
// fires; adapters feed booleans in as checkbox presence rather than a
// raw string, so a boolean field never reaches here through the
// normal form path.
func (f Field) ParseFormValue(raw string) any {
	if raw == "" {
		return nil
	}
	switch f.Type {
	case FieldTypeInteger:
		if n, err := strconv.Atoi(raw); err == nil {
			return n
		}
		return raw
	case FieldTypeDecimal:
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			return n
		}
		return raw
	default:
		return raw
	}
}

// structFieldOrMapValue reads a struct field or map key named `name`
// off obj (dereferencing pointers), used by both Field.GetValue and
// Relation.GetValue's default (Get-less) lookup.
func structFieldOrMapValue(obj any, name string) (any, bool) {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Map:
		mv := v.MapIndex(reflect.ValueOf(name))
		if !mv.IsValid() {
			return nil, false
		}
		return mv.Interface(), true
	case reflect.Struct:
		fv := v.FieldByName(name)
		if !fv.IsValid() {
			return nil, false
		}
		return fv.Interface(), true
	default:
		return nil, false
	}
}

// Validate runs the Required check and any configured Validators,
// returning human-readable error messages.
func (f Field) Validate(value any) []string {
	var errs []string
	if f.Required && isZero(value) {
		errs = append(errs, f.Label+" is required.")
		return errs
	}
	for _, validator := range f.Validators {
		if err := validator(value); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return errs
}

func isZero(value any) bool {
	if value == nil {
		return true
	}
	if s, ok := value.(string); ok {
		return s == ""
	}
	return reflect.ValueOf(value).IsZero()
}
