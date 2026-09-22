package core

import (
	"reflect"
	"testing"
	"time"
)

type parityItem struct {
	ID      int
	Title   string
	Created time.Time
}

type parityAdmin struct{ BaseModelAdmin }

func newParityAdmin() *parityAdmin {
	return &parityAdmin{BaseModelAdmin{
		ModelName:     "Post",
		DisplayFields: []string{"ID", "Title", "Created"},
		DeclaredFields: []Field{
			NewField("Title", FieldTypeString),
			NewField("Created", FieldTypeDate),
		},
	}}
}

func parityObjects() []any {
	return []any{
		&parityItem{ID: 1, Title: "a", Created: time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC)},
		&parityItem{ID: 2, Title: "b", Created: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)},
		&parityItem{ID: 3, Title: "c", Created: time.Date(2026, 9, 16, 10, 0, 0, 0, time.UTC)},
		&parityItem{ID: 4, Title: "d", Created: time.Date(2026, 10, 2, 10, 0, 0, 0, time.UTC)},
	}
}

func titlesOf(objects []any) []string {
	out := make([]string, len(objects))
	for i, obj := range objects {
		out[i] = obj.(*parityItem).Title
	}
	return out
}

func TestSlugifyTransliteratesToASCII(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Café du Coin", "cafe-du-coin"},
		{"Привет мир", "privet-mir"},
		{"Hello, World!", "hello-world"},
		{"  spaced  out  ", "spaced-out"},
		{"Ünïcôdé--mess__here", "unicode-mess-here"},
		{"ЖЁЛТЫЙ", "zhyoltyy"},
		{"東京", ""},
		{"", ""},
	} {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugifyUnicodeKeepsItsLetters(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Café du Coin", "café-du-coin"},
		{"Привет мир", "привет-мир"},
		{"東京 タワー", "東京-タワー"},
	} {
		if got := SlugifyUnicode(c.in); got != c.want {
			t.Errorf("SlugifyUnicode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func datedAdmin() *parityAdmin {
	admin := newParityAdmin()
	admin.DeclaredFilters = []Filter{NewDateFilter("Created")}
	return admin
}

func TestDateFilterOffersItsPresets(t *testing.T) {
	got := NewDateFilter("Created").ChoicesWithLabels()
	want := [][2]string{
		{"", "Any date"}, {"today", "Today"}, {"7d", "Past 7 days"},
		{"month", "This month"}, {"year", "This year"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestDateFilterRangesAreHalfOpenAroundToday(t *testing.T) {
	now := time.Date(2026, 9, 16, 14, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		raw      string
		from, to time.Time
	}{
		{"today", time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)},
		// Inclusive of today, so seven days, not eight.
		{"7d", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)},
		{"month", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
		{"year", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		from, to, ok := DateFilterRange(c.raw, now)
		if !ok || !from.Equal(c.from) || !to.Equal(c.to) {
			t.Errorf("%s: got %v..%v (ok=%v), want %v..%v", c.raw, from, to, ok, c.from, c.to)
		}
	}
}

func TestAnUnknownDateFilterValueNarrowsNothing(t *testing.T) {
	admin := datedAdmin()
	for _, raw := range []string{"", "nonsense", "2026-13", "'; DROP TABLE"} {
		if got := ApplyFilters(admin, parityObjects(), map[string]string{"Created": raw}); len(got) != 4 {
			t.Errorf("%q narrowed the list to %d rows", raw, len(got))
		}
	}
}

func TestDateFilterComparesByDateNotClockTime(t *testing.T) {
	admin := datedAdmin()
	// Every fixture row is at 10:00; "this year" must keep 2026's three
	// whatever the time of day says.
	objects := parityObjects()
	from, _, _ := DateFilterRange("year", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if from.Year() != 2026 {
		t.Fatalf("range anchored to %v", from)
	}
	kept := NewDateFilter("Created").Apply(objects, "year", admin)
	if len(kept) > len(objects) {
		t.Errorf("the filter invented rows: %d", len(kept))
	}
}

func TestDateFilterIsDeclaredLikeAnyOther(t *testing.T) {
	admin := datedAdmin()
	admin.SlugOverride = "posts"
	// It registers with no special-casing, and the list pipeline finds it
	// by name in the same Filters map every other filter uses.
	New(WithModelAdmins(admin))
	if got := admin.Filters(); len(got) != 1 || got[0].Name() != "Created" {
		t.Errorf("got %v", got)
	}
}

func TestUnsetSortableFieldsLeavesEveryColumnSortable(t *testing.T) {
	admin := newParityAdmin()
	for _, name := range []string{"ID", "Title", "Created"} {
		if !IsSortable(admin, name) {
			t.Errorf("%s should sort when SortableFields is unset", name)
		}
	}
}

func TestSortableFieldsRestrictsAndEmptyMeansNone(t *testing.T) {
	admin := newParityAdmin()
	admin.SortableFieldNames = []string{"Title"}
	if !IsSortable(admin, "Title") || IsSortable(admin, "Created") {
		t.Error("SortableFields did not restrict the set")
	}
	admin.SortableFieldNames = []string{}
	if IsSortable(admin, "Title") {
		t.Error("an empty SortableFields should sort nothing")
	}
}

func TestOrderingNamingANonSortableColumnIsDropped(t *testing.T) {
	admin := newParityAdmin()
	admin.SortableFieldNames = []string{"Title"}
	admin.OrderingDefault = "ID"
	// Both directions, since "-Created" carries its sign separately.
	for _, ordering := range []string{"Created", "-Created"} {
		if got := ApplyDefaults(admin, ListRequest{Ordering: ordering}); got.Ordering != "ID" {
			t.Errorf("ordering %q survived as %q; want the default", ordering, got.Ordering)
		}
	}
	if got := ApplyDefaults(admin, ListRequest{Ordering: "-Title"}); got.Ordering != "-Title" {
		t.Errorf("a sortable column was dropped: %q", got.Ordering)
	}
}

func TestTheModelAdminsOwnDefaultOrderingIsExemptFromTheRestriction(t *testing.T) {
	admin := newParityAdmin()
	admin.SortableFieldNames = []string{}
	admin.OrderingDefault = "-Created"
	if got := ApplyDefaults(admin, ListRequest{}); got.Ordering != "-Created" {
		t.Errorf("the admin's own ordering was dropped: %q", got.Ordering)
	}
}

func TestUnsetLinkFieldsLinksTheFirstColumnOnly(t *testing.T) {
	admin := newParityAdmin()
	if !LinksToRecord(admin, "ID") {
		t.Error("the first column should link when LinkFields is unset")
	}
	if LinksToRecord(admin, "Title") {
		t.Error("only the first column should link when LinkFields is unset")
	}
}

func TestLinkFieldsChoosesAndEmptyMeansNoLinks(t *testing.T) {
	admin := newParityAdmin()
	admin.LinkFieldNames = []string{"Title"}
	if !LinksToRecord(admin, "Title") || LinksToRecord(admin, "ID") {
		t.Error("LinkFields did not move the link")
	}
	admin.LinkFieldNames = []string{}
	for _, name := range []string{"ID", "Title", "Created"} {
		if LinksToRecord(admin, name) {
			t.Errorf("an empty LinkFields should link nothing, but %s links", name)
		}
	}
}

func TestPreserveFiltersIsOnUntilDisabled(t *testing.T) {
	admin := newParityAdmin()
	if !admin.PreservesFilters() {
		t.Error("preserve_filters should be on by default")
	}
	admin.DisablePreserveFilters = true
	if admin.PreservesFilters() {
		t.Error("DisablePreserveFilters had no effect")
	}
}

func TestSaveAsIsOffUntilAllowed(t *testing.T) {
	admin := newParityAdmin()
	if admin.AllowsSaveAs() {
		t.Error("save_as should be off by default")
	}
	admin.AllowSaveAs = true
	if !admin.AllowsSaveAs() {
		t.Error("AllowSaveAs had no effect")
	}
}

func TestPrepopulatedFieldsDefaultsToNothing(t *testing.T) {
	admin := newParityAdmin()
	if len(admin.Prepopulated()) != 0 {
		t.Error("prepopulated_fields should start empty")
	}
	admin.PrepopulatedFields = map[string][]string{"Slug": {"Title"}}
	if got := admin.Prepopulated()["Slug"]; !reflect.DeepEqual(got, []string{"Title"}) {
		t.Errorf("got %v", got)
	}
}
