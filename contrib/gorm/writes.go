package gorm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

func (a *ModelAdmin[T]) Create(ctx context.Context, data map[string]any) (any, error) {
	obj := new(T)
	if _, err := a.assign(ctx, obj, data); err != nil {
		return nil, err
	}
	explicitZeros := a.explicitZeros(obj, data)
	err := a.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if err := tx.Omit(clause.Associations).Create(obj).Error; err != nil {
			return err
		}
		if len(explicitZeros) == 0 {
			return nil
		}
		return tx.Model(obj).UpdateColumns(explicitZeros).Error
	})
	if err != nil {
		return nil, a.writeError(ctx, err, false)
	}
	return a.reload(ctx, obj)
}

// explicitZeros are the submitted false, 0 and the like that land on a
// column with a default. GORM's insert replaces a zero value with the
// column default even when the column is selected, so they are written
// again after it. An empty submission is left to the default.
func (a *ModelAdmin[T]) explicitZeros(obj *T, data map[string]any) map[string]any {
	zeros := make(map[string]any)
	value := reflect.ValueOf(obj).Elem()
	for name, submitted := range data {
		column := a.column(name)
		if column == nil || !column.HasDefaultValue || submitted == nil || submitted == "" {
			continue
		}
		if field := value.FieldByIndex(column.StructField.Index); field.IsZero() {
			zeros[column.DBName] = field.Interface()
		}
	}
	return zeros
}

func (a *ModelAdmin[T]) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	current, err := a.load(ctx, a.GetPK(obj))
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, &core.RecordFormError{Errors: map[string][]string{"": {core.T(ctx, "The record no longer exists.")}}}
	}
	columns, err := a.assign(ctx, current, data)
	if err != nil {
		return nil, err
	}
	if len(columns) > 0 {
		if err := a.db.WithContext(ctx).Model(current).Select("*").Omit(clause.Associations).Updates(current).Error; err != nil {
			return nil, a.writeError(ctx, err, false)
		}
	}
	return a.reload(ctx, current)
}

func (a *ModelAdmin[T]) Delete(ctx context.Context, obj any) error {
	current, err := a.load(ctx, a.GetPK(obj))
	if err != nil || current == nil {
		return err
	}
	var links []string
	for name, rel := range a.schema.Relationships.Relations {
		if rel.Type == schema.Many2Many {
			links = append(links, name)
		}
	}
	slices.Sort(links)
	err = a.db.WithContext(ctx).Transaction(func(tx *gormdb.DB) error {
		if len(links) > 0 {
			tx = tx.Select(links)
		}
		return tx.Delete(current).Error
	})
	if err != nil {
		return a.writeError(ctx, err, true)
	}
	return nil
}

func (a *ModelAdmin[T]) reload(ctx context.Context, obj *T) (any, error) {
	fresh, err := a.load(ctx, a.GetPK(obj))
	if err != nil {
		return nil, err
	}
	if fresh == nil {
		return nil, fmt.Errorf("%s: the saved record %v could not be read back", a.Slug(), a.GetPK(obj))
	}
	return fresh, nil
}

// assign sets each submitted value on obj, converted to its column's type,
// and answers the columns it set. A belongs-to field writes its foreign key.
func (a *ModelAdmin[T]) assign(ctx context.Context, obj *T, data map[string]any) ([]string, error) {
	target := reflect.ValueOf(obj).Elem()
	errs := make(map[string][]string)
	var columns []string
	for _, name := range slices.Sorted(maps.Keys(data)) {
		value := data[name]
		column := a.column(name)
		if rel := a.relationship(name); rel != nil && rel.Type == schema.BelongsTo {
			column = rel.References[0].ForeignKey
			if related := reflect.ValueOf(value); related.Kind() == reflect.Pointer && !related.IsNil() && related.Elem().Kind() == reflect.Struct {
				value = related.Elem().FieldByIndex(rel.References[0].PrimaryKey.StructField.Index).Interface()
			}
		}
		if column == nil {
			continue
		}
		converted, err := convert(value, column.FieldType)
		if err != nil {
			errs[name] = append(errs[name], core.T(ctx, "Enter a valid value."))
			continue
		}
		target.FieldByIndex(column.StructField.Index).Set(converted)
		columns = append(columns, column.DBName)
	}
	if len(errs) > 0 {
		return nil, &core.RecordFormError{Errors: errs}
	}
	return columns, nil
}

// writeError turns a constraint the database enforced into messages for
// the user. Anything else is the server's problem, and stays an error.
func (a *ModelAdmin[T]) writeError(ctx context.Context, err error, deleting bool) error {
	translated := err
	if translator, ok := a.db.Dialector.(gormdb.ErrorTranslator); ok {
		translated = translator.Translate(err)
	}
	var message string
	switch {
	case errors.Is(translated, gormdb.ErrDuplicatedKey):
		message = core.T(ctx, "A record with these values already exists.")
	case isForeignKeyViolation(translated) && deleting:
		message = core.T(ctx, "Other records still refer to this one.")
	case isForeignKeyViolation(translated):
		message = core.T(ctx, "The related record does not exist.")
	case errors.Is(translated, gormdb.ErrCheckConstraintViolated):
		message = core.T(ctx, "These values are not allowed.")
	default:
		return err
	}
	return &core.RecordFormError{Errors: map[string][]string{"": {message}}}
}

// isForeignKeyViolation also reads the driver's own text: SQLite reports
// some violations with an extended code its dialect does not translate,
// and SQLite, PostgreSQL and MySQL all name the constraint in the message.
func isForeignKeyViolation(err error) bool {
	return errors.Is(err, gormdb.ErrForeignKeyViolated) || strings.Contains(strings.ToLower(err.Error()), "foreign key constraint")
}
