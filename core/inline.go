package core

import (
	"context"
	"fmt"
)

// Inline layout -- presentation-only, not behavioral, so one Inline
// type covers both (see the Inline doc comment below).
const (
	InlineLayoutStacked = "stacked"
	InlineLayoutTabular = "tabular"
)

// Inline is a reverse-relation admin declaration: lets a parent
// ModelAdmin manage/display a child ModelAdmin's records that point
// back at it via one FK/OneToOne field, Django-admin
// TabularInline/StackedInline style. See docs/inlines.md.
//
// Layout is presentation-only, not behavioral, so there is one Inline
// struct with a Layout discriminator, not two structurally different
// types -- NewStackedInline/NewTabularInline are just layout-preset
// constructors.
type Inline struct {
	Child   string // target (child) ModelAdmin slug
	FKField string // name of the field on the child pointing back at this parent
	Layout  string
	// Label, if "" (default), is derived by the adapter (needs the
	// registry) from the child's own VerboseName.
	Label string
}

// InlineOption configures an Inline built with NewStackedInline/NewTabularInline
// -- mirrors PageOption's pattern in core/page.go.
type InlineOption func(*Inline)

func WithInlineLabel(label string) InlineOption {
	return func(i *Inline) { i.Label = label }
}

func newInline(child, fkField, layout string, opts ...InlineOption) Inline {
	i := Inline{Child: child, FKField: fkField, Layout: layout}
	for _, opt := range opts {
		opt(&i)
	}
	return i
}

func NewStackedInline(child, fkField string, opts ...InlineOption) Inline {
	return newInline(child, fkField, InlineLayoutStacked, opts...)
}

func NewTabularInline(child, fkField string, opts ...InlineOption) Inline {
	return newInline(child, fkField, InlineLayoutTabular, opts...)
}

// FilterInlineChildren returns childAdmin's records (from its own,
// unfiltered GetQueryset) whose fkField value -- the related object
// itself, per Relation.GetValue -- has a PK string-equal to parentPK
// on parentAdmin. Mirrors core/query.go's in-memory
// filter-the-whole-queryset convention; PKs compare as strings
// (stringify, filter.go) to dodge int/string mismatches.
func FilterInlineChildren(ctx context.Context, childAdmin ModelAdmin, fkField string, parentAdmin ModelAdmin, parentPK any) ([]any, error) {
	field, ok := childAdmin.Field(fkField)
	if !ok {
		return nil, fmt.Errorf("polyadmin: child %q has no field %q", childAdmin.Slug(), fkField)
	}
	queryset, err := childAdmin.GetQueryset(ctx)
	if err != nil {
		return nil, err
	}
	items, _ := queryset.([]any)
	parentPKStr := stringify(parentPK)
	out := make([]any, 0, len(items))
	for _, obj := range items {
		related := field.GetValue(obj)
		if IsNil(related) {
			continue
		}
		if stringify(parentAdmin.GetPK(related)) == parentPKStr {
			out = append(out, obj)
		}
	}
	return out, nil
}
