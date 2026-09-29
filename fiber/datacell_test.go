package fiber

import (
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestDataCellHTMLMatchesThePythonContract(t *testing.T) {
	cases := []struct {
		column core.Column
		value  any
		want   string
	}{
		{core.Column{Key: "a", Empty: "never"}, nil, `<span class="` + mustUI("badge", "secondary") + `">never</span>`},
		{core.Column{Key: "a"}, "", `<span class="` + mustUI("text", "placeholder") + `">—</span>`},
		{core.Column{Key: "a", Format: "number"}, 1234, `<span data-format="decimal" data-value="1234">1234</span>`},
		{core.Column{Key: "a", Format: "number"}, 1234.0, `<span data-format="decimal" data-value="1234">1234</span>`},
		{core.Column{Key: "a", Format: "percent"}, 87.25, `<span data-format="decimal" data-value="87.2">87.2</span>%`},
		{core.Column{Key: "a", Format: "share"}, map[string]any{"count": 0.0, "percentage": 0.0}, `<span data-format="decimal" data-value="0">0</span>`},
		{core.Column{Key: "a", Format: "share"}, map[string]any{"count": 12.0, "percentage": 34.5},
			`<span data-format="decimal" data-value="12">12</span> <span class="` + mustUI("text", "muted") + `">(<span data-format="decimal" data-value="34.5">34.5</span>%)</span>`},
		{core.Column{Key: "a", Format: "datetime"}, time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC),
			`<time datetime="2026-09-01T10:30:00Z" data-format="datetime">2026-09-01T10:30:00Z</time>`},
		{core.Column{Key: "a", Format: "datetime"}, "2026-09-01T10:30:00Z",
			`<time datetime="2026-09-01T10:30:00Z" data-format="datetime">2026-09-01T10:30:00Z</time>`},
		{core.Column{Key: "a"}, "<b>", "&lt;b&gt;"},
	}
	for i, c := range cases {
		if got := string(dataCellHTML(c.column, c.value)); got != c.want {
			t.Fatalf("case %d: got %s want %s", i, got, c.want)
		}
	}
}

func TestDataCellTones(t *testing.T) {
	tones := []core.Tone{{Min: 80, Variant: "success"}, {Min: 50, Variant: "warning"}, {Min: 0, Variant: "danger"}}
	column := core.Column{Key: "rate", Format: "percent", Tones: tones}
	cases := map[float64]string{
		87.5: `<span class="` + mustUI("badge", "success") + `"><span data-format="decimal" data-value="87.5">87.5</span>%</span>`,
		50:   `<span class="` + mustUI("badge", "warning") + `"><span data-format="decimal" data-value="50.0">50.0</span>%</span>`,
		12:   `<span class="` + mustUI("badge", "danger") + `"><span data-format="decimal" data-value="12.0">12.0</span>%</span>`,
	}
	for value, want := range cases {
		if got := string(dataCellHTML(column, value)); got != want {
			t.Fatalf("%v: got %s", value, got)
		}
	}
	plain := core.Column{Key: "n", Format: "number", Tones: []core.Tone{{Min: 10, Variant: "success"}}}
	if got := string(dataCellHTML(plain, 3)); got != `<span data-format="decimal" data-value="3">3</span>` {
		t.Fatal(got)
	}
}
