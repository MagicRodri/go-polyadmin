package gorm

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/MagicRodri/go-polyadmin/core"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// ModelAdmin serves a GORM model: search, filters, ordering and paging
// run in SQL (ListPage), and writes go through GORM, so the model's hooks
// fire for every create, edit, bulk edit and delete.
type ModelAdmin[T any] struct {
	core.BaseModelAdmin
	db             *gormdb.DB
	schema         *schema.Schema
	defaultFilters map[string]any
}

type options struct {
	defaultFilters map[string]any
}

type Option func(*options)

// WithDefaultFilters applies field = value until the user picks that
// filter, "All" included.
func WithDefaultFilters(filters map[string]any) Option {
	return func(o *options) { o.defaultFilters = filters }
}

func New[T any](db *gormdb.DB, base core.BaseModelAdmin, opts ...Option) (*ModelAdmin[T], error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s: %s is not a struct type", base.Slug(), reflect.TypeFor[T]())
	}
	parsed, err := schema.Parse(new(T), &sync.Map{}, db.NamingStrategy)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", base.Slug(), err)
	}
	if len(parsed.PrimaryFields) != 1 {
		return nil, fmt.Errorf("%s: the model needs exactly one primary key, it has %d", base.Slug(), len(parsed.PrimaryFields))
	}
	admin := &ModelAdmin[T]{BaseModelAdmin: base, db: db, schema: parsed, defaultFilters: o.defaultFilters}
	admin.DeclaredFields = append(slices.Clone(base.DeclaredFields), admin.derivedFields()...)
	if err := admin.check(); err != nil {
		return nil, fmt.Errorf("%s: %w", base.Slug(), err)
	}
	return admin, nil
}

func (a *ModelAdmin[T]) check() error {
	for _, filter := range a.DeclaredFilters {
		switch filter.(type) {
		case core.BooleanFilter, core.ChoiceFilter, core.EmptyFilter, core.DateFilter, core.RelationFilter:
		default:
			return fmt.Errorf("filter %q (%T) has no SQL translation", filter.Name(), filter)
		}
		if a.column(filter.Name()) == nil && a.relationship(filter.Name()) == nil {
			return fmt.Errorf("filter %q names no column or relationship", filter.Name())
		}
	}
	for _, name := range slices.Sorted(maps.Keys(a.defaultFilters)) {
		if a.column(name) == nil {
			return fmt.Errorf("default filter %q names no column", name)
		}
	}
	for _, name := range a.FormFields() {
		if rel := a.relationship(name); rel != nil && rel.Type != schema.BelongsTo {
			return fmt.Errorf("form field %q is a %s relationship, which is not writable", name, rel.Type)
		}
	}
	return nil
}

func (a *ModelAdmin[T]) derivedFields() []core.Field {
	seen := make(map[string]bool, len(a.DeclaredFields))
	for _, f := range a.DeclaredFields {
		seen[f.Name] = true
	}
	var derived []core.Field
	names := slices.Concat(a.DisplayFields, a.FormFields(), a.SearchFieldNames, a.DetailFieldNames, a.BulkEditFieldNames)
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if column := a.column(name); column != nil {
			derived = append(derived, deriveField(column))
		}
	}
	return derived
}

func (a *ModelAdmin[T]) DefaultFilters() map[string]any { return a.defaultFilters }

func (a *ModelAdmin[T]) GetPK(obj any) any {
	v := reflect.ValueOf(obj)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	return v.FieldByIndex(a.pkField().StructField.Index).Interface()
}

func (a *ModelAdmin[T]) column(name string) *schema.Field {
	f := a.schema.FieldsByName[name]
	if f == nil || f.DBName == "" {
		return nil
	}
	return f
}

func (a *ModelAdmin[T]) relationship(name string) *schema.Relationship {
	return a.schema.Relationships.Relations[name]
}

func (a *ModelAdmin[T]) pkField() *schema.Field { return a.schema.PrimaryFields[0] }

func (a *ModelAdmin[T]) col(f *schema.Field) clause.Column {
	return clause.Column{Table: clause.CurrentTable, Name: f.DBName}
}
