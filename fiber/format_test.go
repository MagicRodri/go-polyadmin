package fiber

import (
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

func renderValue(t *testing.T, fieldType core.FieldType, value any) string {
	t.Helper()
	r, err := NewRenderer(core.New(), "/admin")
	if err != nil {
		t.Fatal(err)
	}
	return string(r.fieldValueHTML(nil, core.NewField("X", fieldType), value, ""))
}

func TestDateRendersAsATimeElement(t *testing.T) {
	got := renderValue(t, core.FieldTypeDate, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	if want := `<time datetime="2026-09-14" data-format="date">2026-09-14</time>`; got != want {
		t.Errorf("got %s", got)
	}
	if got := renderValue(t, core.FieldTypeDate, "2026-09-14"); !strings.Contains(got, `datetime="2026-09-14"`) {
		t.Errorf("an ISO date string is accepted too: %s", got)
	}
}

func TestDateTimeKeepsItsOffset(t *testing.T) {
	loc := time.FixedZone("x", 2*60*60)
	got := renderValue(t, core.FieldTypeDateTime, time.Date(2026, 9, 14, 10, 30, 0, 0, loc))
	if want := `<time datetime="2026-09-14T10:30:00+02:00" data-format="datetime">2026-09-14T10:30:00+02:00</time>`; got != want {
		t.Errorf("got %s", got)
	}
	if got := renderValue(t, core.FieldTypeDateTime, "2026-09-14T10:30:00"); !strings.Contains(got, `datetime="2026-09-14T10:30:00"`) {
		t.Errorf("a naive string stays naive: %s", got)
	}
}

func TestDecimalCarriesItsRawValue(t *testing.T) {
	if got := renderValue(t, core.FieldTypeDecimal, 1234.5); got != `<span data-format="decimal" data-value="1234.5">1234.5</span>` {
		t.Errorf("got %s", got)
	}
}

func TestDecimalIsFixedPointNotScientificNotation(t *testing.T) {
	// fmt.Sprint(1e21) is "1e+21"; the data-value/text must stay
	// fixed-point so the browser-side formatter can count fraction digits.
	if got := renderValue(t, core.FieldTypeDecimal, 1e21); got != `<span data-format="decimal" data-value="1000000000000000000000">1000000000000000000000</span>` {
		t.Errorf("got %s", got)
	}
}

func TestIntegersAreNotFormatted(t *testing.T) {
	if got := renderValue(t, core.FieldTypeInteger, 1234); got != "1234" {
		t.Errorf("got %s", got)
	}
}

func TestUnparseableDateFallsBackToText(t *testing.T) {
	if got := renderValue(t, core.FieldTypeDate, "someday"); got != "someday" {
		t.Errorf("got %s", got)
	}
}

func renderInput(t *testing.T, fieldType core.FieldType, value any) string {
	t.Helper()
	r, err := NewRenderer(core.New(), "/admin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.formInputHTML("/admin", core.NewField("X", fieldType), value, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	return string(got)
}

// A date or datetime-local input discards any value that isn't its own
// ISO form, so a time.Time must be filled in as exactly that -- and the
// zero time, which no host means as a real date, as empty.
func TestDateInputsAreFilledWithTheirISOForm(t *testing.T) {
	loc := time.FixedZone("x", 2*60*60)
	cases := []struct {
		fieldType core.FieldType
		value     any
		want      string
	}{
		{core.FieldTypeDate, time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC), `value="2019-03-01"`},
		{core.FieldTypeDateTime, time.Date(2019, 3, 1, 9, 5, 30, 0, loc), `value="2019-03-01T09:05"`},
		{core.FieldTypeDate, time.Time{}, `value=""`},
		{core.FieldTypeDateTime, time.Time{}, `value=""`},
		{core.FieldTypeDate, "2019-03-01", `value="2019-03-01"`},
	}
	for _, c := range cases {
		if got := renderInput(t, c.fieldType, c.value); !strings.Contains(got, c.want) {
			t.Errorf("%s %v: want %s in\n%s", c.fieldType, c.value, c.want, got)
		}
	}
}

func TestZeroTimeRendersAsEmpty(t *testing.T) {
	for _, fieldType := range []core.FieldType{core.FieldTypeDate, core.FieldTypeDateTime} {
		if got := renderValue(t, fieldType, time.Time{}); strings.Contains(got, "0001") || !strings.Contains(got, core.DefaultEmptyValue) {
			t.Errorf("%s: the zero time should show as empty, got %s", fieldType, got)
		}
	}
}

func TestInlineDateCellIsFilledWithItsISOForm(t *testing.T) {
	got := string(breadcrumbRenderer(t, newTestUserAdmin()).inlineTableCellHTML("/admin", core.NewField("X", core.FieldTypeDate), time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC), nil, nil))
	if !strings.Contains(got, `value="2019-03-01"`) {
		t.Errorf("got %s", got)
	}
}

func TestDatetimeInputPadsYearsBelowAThousand(t *testing.T) {
	got := inputValue(core.FieldTypeDateTime, time.Date(99, 3, 1, 10, 30, 0, 0, time.UTC))
	if got != "0099-03-01T10:30" {
		t.Errorf("got %q, want %q", got, "0099-03-01T10:30")
	}
}
