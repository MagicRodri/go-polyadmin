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
