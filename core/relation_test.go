package core

import "testing"

type Organization struct {
	ID   int
	Name string
}

type Item struct {
	ID           int
	Organization *Organization
	Tags         []any
}

func TestRelationGetValueDefaultsToStructField(t *testing.T) {
	relation := Relation{Name: "Organization", Target: "organizations"}
	org := &Organization{ID: 1, Name: "Acme"}
	if relation.GetValue(&Item{Organization: org}) != org {
		t.Fatalf("expected org")
	}
}

func TestRelationGetValueUsesCustomGetter(t *testing.T) {
	relation := Relation{Name: "Organization", Target: "organizations", GetRelated: func(obj any) any { return "custom" }}
	if relation.GetValue(&Item{}) != "custom" {
		t.Fatalf("expected custom")
	}
}

func TestForeignKeyFieldGetValueReturnsRelatedObject(t *testing.T) {
	relation := Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}
	field := NewField("Organization", FieldTypeForeignKey, WithRelation(relation))
	org := &Organization{ID: 1, Name: "Acme"}
	if field.GetValue(&Item{Organization: org}) != org {
		t.Fatalf("expected org")
	}
}

func TestForeignKeyFieldGetValueNilWhenUnset(t *testing.T) {
	relation := Relation{Name: "Organization", Target: "organizations"}
	field := NewField("Organization", FieldTypeForeignKey, WithRelation(relation))
	if field.GetValue(&Item{}) != nil {
		t.Fatalf("expected nil")
	}
}

func TestManyToManyFieldDefaultsToEmptySlice(t *testing.T) {
	relation := Relation{Name: "Tags", Target: "tags", Cardinality: CardinalityMany}
	field := NewField("Tags", FieldTypeManyToMany, WithRelation(relation))
	value, ok := field.GetValue(&Item{}).([]any)
	if !ok || len(value) != 0 {
		t.Fatalf("got %v", field.GetValue(&Item{}))
	}
}

func TestManyToManyFieldReturnsRelatedObjects(t *testing.T) {
	relation := Relation{Name: "Tags", Target: "tags", Cardinality: CardinalityMany}
	field := NewField("Tags", FieldTypeManyToMany, WithRelation(relation))
	tags := []any{"urgent", "billing"}
	value, ok := field.GetValue(&Item{Tags: tags}).([]any)
	if !ok || len(value) != 2 {
		t.Fatalf("got %v", value)
	}
}
