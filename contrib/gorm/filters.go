package gorm

import (
	"reflect"
	"strings"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

var matchNothing = clause.Expr{SQL: "1 = 0"}

// linkedAlias names the subquery's table, so a model linking to itself
// (a manager's reports) still tells the inner rows from the outer one.
const linkedAlias = "linked"

// filterClause mirrors the in-memory Apply of each filter it translates,
// so a list reads the same whichever side resolves it.
func (a *ModelAdmin[T]) filterClause(filter core.Filter, raw string) clause.Expression {
	switch filter.(type) {
	case core.RelationFilter:
		return a.relationClause(filter.Name(), raw)
	case core.EmptyFilter:
		return a.emptyClause(filter.Name(), raw)
	}
	column := a.column(filter.Name())
	if column == nil {
		return nil
	}
	switch filter.(type) {
	case core.BooleanFilter:
		if truthy[strings.ToLower(raw)] {
			return clause.Eq{Column: a.col(column), Value: true}
		}
		return clause.Or(clause.Eq{Column: a.col(column), Value: false}, clause.Eq{Column: a.col(column), Value: nil})
	case core.ChoiceFilter:
		return a.equals(column, raw)
	case core.DateFilter:
		from, to, ok := core.DateFilterRange(raw, time.Now())
		if !ok {
			return nil
		}
		if strings.EqualFold(column.TagSettings["TYPE"], "date") {
			const day = "2006-01-02"
			return clause.And(clause.Gte{Column: a.col(column), Value: from.Format(day)}, clause.Lt{Column: a.col(column), Value: to.Format(day)})
		}
		return clause.And(clause.Gte{Column: a.col(column), Value: from}, clause.Lt{Column: a.col(column), Value: to})
	}
	return nil
}

func (a *ModelAdmin[T]) equals(column *schema.Field, raw string) clause.Expression {
	value, err := convert(raw, column.IndirectFieldType)
	if err != nil {
		return matchNothing
	}
	return clause.Eq{Column: a.col(column), Value: value.Interface()}
}

func (a *ModelAdmin[T]) relationClause(name, raw string) clause.Expression {
	rel := a.relationship(name)
	if rel == nil {
		return a.equals(a.column(name), raw)
	}
	if rel.Type == schema.BelongsTo {
		return a.equals(rel.References[0].ForeignKey, raw)
	}
	value, err := convert(raw, a.targetKey(rel).IndirectFieldType)
	if err != nil {
		return matchNothing
	}
	return a.relatedExists(rel, value.Interface())
}

func (a *ModelAdmin[T]) emptyClause(name, raw string) clause.Expression {
	if raw != core.EmptyFilterEmpty && raw != core.EmptyFilterNotEmpty {
		return nil
	}
	var empty, notEmpty clause.Expression
	if rel := a.relationship(name); rel != nil {
		if rel.Type == schema.BelongsTo {
			empty = clause.Eq{Column: a.col(rel.References[0].ForeignKey), Value: nil}
			notEmpty = clause.Neq{Column: a.col(rel.References[0].ForeignKey), Value: nil}
		} else {
			notEmpty = a.relatedExists(rel, nil)
			empty = clause.Not(notEmpty)
		}
	} else {
		column := a.column(name)
		empty = clause.Eq{Column: a.col(column), Value: nil}
		if column.IndirectFieldType.Kind() == reflect.String {
			empty = clause.Or(empty, clause.Eq{Column: a.col(column), Value: ""})
		}
		notEmpty = clause.Not(empty)
	}
	if raw == core.EmptyFilterEmpty {
		return empty
	}
	return notEmpty
}

// targetKey is the related model's primary key as the link stores it.
func (a *ModelAdmin[T]) targetKey(rel *schema.Relationship) *schema.Field {
	if rel.Type == schema.Many2Many {
		for _, ref := range rel.References {
			if !ref.OwnPrimaryKey {
				return ref.PrimaryKey
			}
		}
	}
	return rel.FieldSchema.PrioritizedPrimaryField
}

// relatedExists is EXISTS over the rows linking a record to rel's target --
// the join table for many-to-many, the child table otherwise -- narrowed
// to one target key when value is not nil.
func (a *ModelAdmin[T]) relatedExists(rel *schema.Relationship, value any) clause.Expression {
	var table string
	var link, match clause.Column
	var own *schema.Field
	if rel.Type == schema.Many2Many {
		table = rel.JoinTable.Table
		for _, ref := range rel.References {
			if ref.OwnPrimaryKey {
				link, own = clause.Column{Table: linkedAlias, Name: ref.ForeignKey.DBName}, ref.PrimaryKey
			} else {
				match = clause.Column{Table: linkedAlias, Name: ref.ForeignKey.DBName}
			}
		}
	} else {
		table = rel.FieldSchema.Table
		ref := rel.References[0]
		link, own = clause.Column{Table: linkedAlias, Name: ref.ForeignKey.DBName}, ref.PrimaryKey
		match = clause.Column{Table: linkedAlias, Name: rel.FieldSchema.PrioritizedPrimaryField.DBName}
	}
	sql := "EXISTS (SELECT 1 FROM ? WHERE ? = ?"
	vars := []any{clause.Table{Name: table, Alias: linkedAlias}, link, a.col(own)}
	if value != nil {
		sql += " AND ? = ?"
		vars = append(vars, match, value)
	}
	return clause.Expr{SQL: sql + ")", Vars: vars}
}
