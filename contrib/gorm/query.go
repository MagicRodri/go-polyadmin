package gorm

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func (a *ModelAdmin[T]) ListPage(ctx context.Context, req core.ListRequest) ([]any, int, error) {
	where := a.where(req)
	scoped := func() *gormdb.DB {
		tx := a.db.WithContext(ctx).Model(new(T))
		for _, condition := range where {
			tx = tx.Where(condition)
		}
		return tx
	}
	var total int64
	if err := scoped().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	tx := a.preloads(scoped())
	for _, order := range a.orderBy(req.Ordering) {
		tx = tx.Order(order)
	}
	offset, limit := req.Window()
	if offset > 0 {
		tx = tx.Offset(offset)
	}
	if limit > 0 {
		tx = tx.Limit(limit)
	}
	var rows []T
	if err := tx.Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	objects := make([]any, len(rows))
	for i := range rows {
		objects[i] = &rows[i]
	}
	return objects, int(total), nil
}

func (a *ModelAdmin[T]) GetQueryset(ctx context.Context) (any, error) {
	objects, _, err := a.ListPage(ctx, core.ListRequest{Unlimited: true})
	return objects, err
}

// GetObject answers nil for a key that does not convert, a row that is
// not there, and a key the database rejects: all read as "not found".
func (a *ModelAdmin[T]) GetObject(ctx context.Context, pk any) (any, error) {
	if pk == nil || pk == "" {
		return nil, nil
	}
	key, err := convert(pk, a.pkField().IndirectFieldType)
	if err != nil {
		return nil, nil
	}
	obj, err := a.load(ctx, key.Interface())
	if err != nil || obj == nil {
		return nil, nil
	}
	return obj, nil
}

func (a *ModelAdmin[T]) load(ctx context.Context, key any) (*T, error) {
	var rows []T
	tx := a.preloads(a.db.WithContext(ctx)).Where(clause.Eq{Column: a.col(a.pkField()), Value: key}).Limit(1)
	if err := tx.Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

func (a *ModelAdmin[T]) preloads(tx *gormdb.DB) *gormdb.DB {
	names := make(map[string]bool)
	for _, name := range slices.Concat(a.ListDisplay(), a.FormFields(), a.DetailFields()) {
		if a.relationship(name) != nil {
			names[name] = true
		}
	}
	for _, name := range slices.Sorted(maps.Keys(names)) {
		tx = tx.Preload(name)
	}
	return tx
}

func (a *ModelAdmin[T]) where(req core.ListRequest) []clause.Expression {
	var conditions []clause.Expression
	if req.Search != "" {
		pattern := "%" + likeEscaper.Replace(strings.ToLower(req.Search)) + "%"
		sql := `LOWER(CAST(? AS TEXT)) LIKE ? ESCAPE '\'`
		if a.db.Dialector.Name() == "mysql" {
			sql = `LOWER(CAST(? AS CHAR)) LIKE ? ESCAPE '\\'`
		}
		var matches []clause.Expression
		for _, name := range a.SearchFields() {
			if column := a.column(name); column != nil {
				matches = append(matches, clause.Expr{SQL: sql, Vars: []any{a.col(column), pattern}})
			}
		}
		if len(matches) > 0 {
			conditions = append(conditions, anyOf(matches...))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(a.defaultFilters)) {
		if _, chosen := req.Filters[name]; !chosen {
			conditions = append(conditions, clause.Eq{Column: a.col(a.column(name)), Value: a.defaultFilters[name]})
		}
	}
	for _, filter := range a.Filters() {
		raw := req.Filters[filter.Name()]
		if raw == "" {
			continue
		}
		if condition := a.filterClause(filter, raw); condition != nil {
			conditions = append(conditions, condition)
		}
	}
	return conditions
}

// anyOf ORs its expressions. A lone expression is returned bare: GORM
// joins a one-element OR group to the conditions before it with OR.
func anyOf(exprs ...clause.Expression) clause.Expression {
	if len(exprs) == 1 {
		return exprs[0]
	}
	return clause.Or(exprs...)
}

func (a *ModelAdmin[T]) orderBy(ordering string) []clause.OrderByColumn {
	if ordering == "" {
		ordering = a.DefaultOrdering()
	}
	pk := clause.OrderByColumn{Column: a.col(a.pkField())}
	column := a.column(strings.TrimPrefix(ordering, "-"))
	if column == nil {
		return []clause.OrderByColumn{pk}
	}
	first := clause.OrderByColumn{Column: a.col(column), Desc: strings.HasPrefix(ordering, "-")}
	if column.PrimaryKey {
		return []clause.OrderByColumn{first}
	}
	return []clause.OrderByColumn{first, pk}
}
