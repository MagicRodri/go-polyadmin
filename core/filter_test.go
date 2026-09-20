package core

import (
	"testing"
	"time"
)

type emptyFixture struct {
	ID    int
	Notes string
	Tags  []string
	Ref   *emptyFixture
	Count int
	Flag  bool
}

func emptyFilterAdmin() *BaseModelAdmin {
	return &BaseModelAdmin{
		ModelName:     "Thing",
		DisplayFields: []string{"ID", "Notes"},
		DeclaredFields: []Field{
			NewField("Notes", FieldTypeString),
			NewField("Tags", FieldTypeString),
			NewField("Ref", FieldTypeString),
			NewField("Count", FieldTypeInteger),
			NewField("Flag", FieldTypeBoolean),
		},
	}
}

func timeZeroForTest() any {
	var zero time.Time
	return zero
}

// TestZeroIsAValueNotAnAbsence is the whole semantic decision in one
// test: somebody chose 0 and false, so neither is empty. Go's plain
// int and bool therefore can never be empty, which is correct -- they
// cannot represent "unset".
func TestZeroIsAValueNotAnAbsence(t *testing.T) {
	for _, c := range []struct {
		name  string
		value any
		empty bool
	}{
		{"nil", nil, true},
		{"empty string", "", true},
		{"empty slice", []string{}, true},
		{"nil pointer", (*emptyFixture)(nil), true},
		{"zero time", timeZeroForTest(), true},
		{"zero int", 0, false},
		{"false", false, false},
		{"the string zero", "0", false},
		{"a word", "hi", false},
	} {
		if got := IsEmptyValue(c.value); got != c.empty {
			t.Errorf("IsEmptyValue(%s) = %v, want %v", c.name, got, c.empty)
		}
	}
}

func TestEmptyFilterKeepsOnlyBlanks(t *testing.T) {
	admin := emptyFilterAdmin()
	blank := &emptyFixture{ID: 1, Notes: ""}
	filled := &emptyFixture{ID: 2, Notes: "something"}
	objects := []any{blank, filled}

	filter := NewEmptyFilter("Notes")
	if got := filter.Apply(objects, EmptyFilterEmpty, admin); len(got) != 1 || got[0] != blank {
		t.Errorf("empty kept %v, want just the blank one", got)
	}
	if got := filter.Apply(objects, EmptyFilterNotEmpty, admin); len(got) != 1 || got[0] != filled {
		t.Errorf("notempty kept %v, want just the filled one", got)
	}
	// An unrecognised value narrows nothing, as every other filter does.
	if got := filter.Apply(objects, "nonsense", admin); len(got) != 2 {
		t.Errorf("a crafted value narrowed the list to %d rows", len(got))
	}
	if got := filter.Apply(objects, "", admin); len(got) != 2 {
		t.Errorf("the empty value narrowed the list to %d rows", len(got))
	}
}

func TestEmptyFilterOffersItsThreeChoices(t *testing.T) {
	got := NewEmptyFilter("Notes").ChoicesWithLabels()
	want := [][2]string{{"", "All"}, {EmptyFilterEmpty, "Empty"}, {EmptyFilterNotEmpty, "Not empty"}}
	if len(got) != len(want) {
		t.Fatalf("got %d choices, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("choice %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestAFilterWithoutAControlKindIsAChoiceList: the capability is
// optional, so every filter that existed before this change keeps
// rendering as it did.
func TestAFilterWithoutAControlKindIsAChoiceList(t *testing.T) {
	for _, f := range []Filter{
		NewBooleanFilter("Flag"),
		NewChoiceFilter("Notes", []string{"a"}),
		NewEmptyFilter("Notes"),
	} {
		if _, declares := f.(FilterControl); declares {
			t.Errorf("%T declares a control kind; it should be a plain choice list", f)
		}
	}
}

func TestACustomRangeIsInclusiveOfBothEnds(t *testing.T) {
	now := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)
	from, to, ok := DateFilterRange("2026-01-01:2026-03-01", now)
	if !ok {
		t.Fatal("a well-formed range was rejected")
	}
	if !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %s, want 2026-01-01", from)
	}
	// Half-open internally, so the inclusive end is the following day.
	if !to.Equal(time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("to = %s, want 2026-03-02 (2026-03-01 inclusive)", to)
	}
}

func TestAnEmptyRangeEndMeansThroughToday(t *testing.T) {
	// Resolved by the parser, not the UI: the JS-off form posts the
	// blank, and a value the parser then rejected would break exactly
	// the path the panel's links exist to protect.
	now := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)
	from, to, ok := DateFilterRange("2026-01-01:", now)
	if !ok {
		t.Fatal("an open end was rejected")
	}
	if !from.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %s", from)
	}
	if !to.Equal(time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("to = %s, want the day after today", to)
	}
}

func TestAMalformedRangeNarrowsNothing(t *testing.T) {
	now := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)
	for _, raw := range []string{
		":2026-03-01",           // no start: unbounded-below has no natural resolution
		"2026-03-01:2026-01-01", // end before start
		"nonsense:2026-03-01",   // unparseable start
		"2026-01-01:nonsense",   // unparseable end
		"2026-01-01",            // no separator: not a range, not a preset
		"01/01/2026:01/03/2026", // wrong layout
	} {
		if _, _, ok := DateFilterRange(raw, now); ok {
			t.Errorf("%q was accepted; a crafted value must narrow nothing", raw)
		}
	}
}

func TestTheRangeValuesComeBackForPrefilling(t *testing.T) {
	from, to, ok := DateFilterRangeValues("2026-01-01:2026-03-01")
	if !ok || from != "2026-01-01" || to != "2026-03-01" {
		t.Errorf("got (%q, %q, %v)", from, to, ok)
	}
	if _, _, ok := DateFilterRangeValues(DateFilterPast7Days); ok {
		t.Error("a preset was reported as a range")
	}
}

func TestDateFilterDeclaresTheRangeControl(t *testing.T) {
	control, ok := any(NewDateFilter("Founded")).(FilterControl)
	if !ok {
		t.Fatal("DateFilter does not declare a control kind")
	}
	if got := control.ControlKind(); got != FilterKindDateRange {
		t.Errorf("ControlKind() = %q, want %q", got, FilterKindDateRange)
	}
}

// TestTheRangeAndThePresetsShareOneParser is the property step 3 was
// built around: one parser means the in-memory path and a ListQuerier
// host cannot disagree about what a value means.
func TestTheRangeAndThePresetsShareOneParser(t *testing.T) {
	now := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)
	preset, presetTo, _ := DateFilterRange(DateFilterToday, now)
	spelled, spelledTo, ok := DateFilterRange("2026-03-15:2026-03-15", now)
	if !ok {
		t.Fatal("a single-day range was rejected")
	}
	if !preset.Equal(spelled) || !presetTo.Equal(spelledTo) {
		t.Errorf("today = [%s, %s), the same day spelled out = [%s, %s)", preset, presetTo, spelled, spelledTo)
	}
}

// TestTheRangeFormFoldsIntoOneFilterValue: the panel's two date inputs
// cannot produce one parameter on their own, so the handler folds them.
// Folding server-side rather than in JS is what keeps the range working
// with scripting off.
func TestTheRangeFormFoldsIntoOneFilterValue(t *testing.T) {
	admin := &BaseModelAdmin{
		ModelName:       "Org",
		DeclaredFilters: []Filter{NewDateFilter("Founded")},
	}
	filters := map[string]string{"Other": "kept"}
	FoldRangeParams(admin, filters, "Founded", "2026-01-01", "2026-03-01")
	if got := filters["Founded"]; got != "2026-01-01:2026-03-01" {
		t.Errorf("Founded = %q, want the joined range", got)
	}
	if filters["Other"] != "kept" {
		t.Error("folding clobbered another filter")
	}
}

func TestTheRangeFormIgnoresAnUndeclaredFilter(t *testing.T) {
	// _range_for arrives from the client, so it names a filter only if
	// the ModelAdmin declared one -- otherwise a crafted form could
	// inject any key into Filters.
	admin := &BaseModelAdmin{ModelName: "Org", DeclaredFilters: []Filter{NewDateFilter("Founded")}}
	filters := map[string]string{}
	FoldRangeParams(admin, filters, "Sneaky", "2026-01-01", "2026-03-01")
	if len(filters) != 0 {
		t.Errorf("an undeclared filter was injected: %v", filters)
	}
}

func TestAnEmptyRangeFormClearsTheFilter(t *testing.T) {
	admin := &BaseModelAdmin{ModelName: "Org", DeclaredFilters: []Filter{NewDateFilter("Founded")}}
	filters := map[string]string{"Founded": "7d"}
	FoldRangeParams(admin, filters, "Founded", "", "")
	if _, still := filters["Founded"]; still {
		t.Error("submitting an empty range left the old value in place")
	}
}

type relTarget struct {
	ID   int
	Name string
}

type relRow struct {
	ID    int
	Org   *relTarget
	Teams []any
}

func relationFilterAdmin() *BaseModelAdmin {
	return &BaseModelAdmin{
		ModelName:     "Row",
		DisplayFields: []string{"ID", "Org"},
		DeclaredFields: []Field{
			NewField("Org", FieldTypeForeignKey, WithRelation(Relation{
				Name: "Org", Target: "orgs", DisplayField: "Name", Cardinality: CardinalityOne,
			})),
			NewField("Teams", FieldTypeManyToMany, WithRelation(Relation{
				Name: "Teams", Target: "orgs", DisplayField: "Name", Cardinality: CardinalityMany,
			})),
		},
	}
}

func TestRelationFilterMatchesTheRelatedPrimaryKey(t *testing.T) {
	admin := relationFilterAdmin()
	acme := &relTarget{ID: 7, Name: "Acme"}
	other := &relTarget{ID: 8, Name: "Other"}
	kept := &relRow{ID: 1, Org: acme}
	dropped := &relRow{ID: 2, Org: other}
	unset := &relRow{ID: 3}
	objects := []any{kept, dropped, unset}

	got := NewRelationFilter("Org").Apply(objects, "7", admin)
	if len(got) != 1 || got[0] != kept {
		t.Errorf("got %v, want just the Acme row", got)
	}
	// An unset relation never matches a chosen pk.
	if got := NewRelationFilter("Org").Apply(objects, "0", admin); len(got) != 0 {
		t.Errorf("an unset relation matched: %v", got)
	}
}

func TestRelationFilterMatchesAnyMemberOfAManyRelation(t *testing.T) {
	admin := relationFilterAdmin()
	platform := &relTarget{ID: 9, Name: "Platform"}
	security := &relTarget{ID: 10, Name: "Security"}
	kept := &relRow{ID: 1, Teams: []any{security, platform}}
	dropped := &relRow{ID: 2, Teams: []any{security}}

	got := NewRelationFilter("Teams").Apply([]any{kept, dropped}, "9", admin)
	if len(got) != 1 || got[0] != kept {
		t.Errorf("got %v, want the row whose Teams include Platform", got)
	}
}

// TestRelationFilterHonoursACustomPKResolver: Apply gets the parent
// ModelAdmin, not the registry, so it cannot call the target's own
// GetPK. defaultPK covers the common case and RelatedPK is the escape
// hatch -- the same shape as Relation.GetRelated, and for the same
// reason.
func TestRelationFilterHonoursACustomPKResolver(t *testing.T) {
	admin := relationFilterAdmin()
	kept := &relRow{ID: 1, Org: &relTarget{ID: 7, Name: "Acme"}}
	dropped := &relRow{ID: 2, Org: &relTarget{ID: 8, Name: "Other"}}

	filter := NewRelationFilter("Org")
	filter.RelatedPK = func(related any) any { return related.(*relTarget).Name }

	got := filter.Apply([]any{kept, dropped}, "Acme", admin)
	if len(got) != 1 || got[0] != kept {
		t.Errorf("got %v, want the row matched by name", got)
	}
}

func TestRelationFilterLeavesItsChoicesToTheAdapter(t *testing.T) {
	// ChoicesWithLabels reaches neither the registry nor the principal,
	// so it cannot enumerate a target it is allowed to show. It offers
	// only "All"; the adapter fills the rest, permission-filtered.
	got := NewRelationFilter("Org").ChoicesWithLabels()
	if len(got) != 1 || got[0] != [2]string{"", "All"} {
		t.Errorf("got %v, want just the All choice", got)
	}
	control, ok := any(NewRelationFilter("Org")).(FilterControl)
	if !ok || control.ControlKind() != FilterKindRelation {
		t.Error("RelationFilter does not declare the relation control kind")
	}
}

func TestRelationFilterWithNoValueNarrowsNothing(t *testing.T) {
	admin := relationFilterAdmin()
	objects := []any{&relRow{ID: 1, Org: &relTarget{ID: 7}}, &relRow{ID: 2}}
	if got := NewRelationFilter("Org").Apply(objects, "", admin); len(got) != 2 {
		t.Errorf("an empty value narrowed the list to %d rows", len(got))
	}
}
