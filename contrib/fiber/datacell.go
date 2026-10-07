package fiber

import (
	"fmt"
	"html"
	"html/template"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
)

var (
	classCellEmptyBadge  = mustUI("badge", "secondary")
	classCellPlaceholder = mustUI("text", "placeholder")
	classCellMuted       = mustUI("text", "muted")
)

func decimalSpan(raw string) string {
	raw = html.EscapeString(raw)
	return `<span data-format="decimal" data-value="` + raw + `">` + raw + `</span>`
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

// dataCellHTML is one DataTable cell, rendered per its column's format --
// the same markup as the Python adapter's data_cell. Numbers carry their
// raw value for theme.html's locale formatter.
func toneFor(column core.Column, value any) string {
	number, ok := toFloat(value)
	if !ok {
		return ""
	}
	best, found := "", false
	var bestMin float64
	for _, tone := range column.Tones {
		if number >= tone.Min && (!found || tone.Min > bestMin) {
			best, bestMin, found = tone.Variant, tone.Min, true
		}
	}
	return best
}

func dataCellHTML(column core.Column, value any) template.HTML {
	cell := plainCellHTML(column, value)
	if value == nil || value == "" || len(column.Tones) == 0 {
		return cell
	}
	if tone := toneFor(column, value); tone != "" {
		return template.HTML(`<span class="`+mustUI("badge", tone)+`">`) + cell + template.HTML(`</span>`)
	}
	return cell
}

func plainCellHTML(column core.Column, value any) template.HTML {
	if value == nil || value == "" {
		if column.Empty != "" {
			return template.HTML(`<span class="` + classCellEmptyBadge + `">` + html.EscapeString(column.Empty) + `</span>`)
		}
		return template.HTML(`<span class="` + classCellPlaceholder + `">—</span>`)
	}
	switch column.Format {
	case "number":
		return template.HTML(decimalSpan(decimalText(value)))
	case "percent":
		if f, ok := toFloat(value); ok {
			return template.HTML(decimalSpan(strconv.FormatFloat(f, 'f', 1, 64)) + "%")
		}
	case "share":
		if m, ok := value.(map[string]any); ok {
			count := m["count"]
			if count == nil {
				count = 0
			}
			cell := decimalSpan(decimalText(count))
			if f, _ := toFloat(count); f != 0 {
				pct, _ := toFloat(m["percentage"])
				cell += ` <span class="` + classCellMuted + `">(` + decimalSpan(strconv.FormatFloat(pct, 'f', 1, 64)) + `%)</span>`
			}
			return template.HTML(cell)
		}
	case "datetime":
		if iso, ok := isoDateTime(value); ok {
			return template.HTML(`<time datetime="` + iso + `" data-format="datetime">` + iso + `</time>`)
		}
	}
	return template.HTML(html.EscapeString(fmt.Sprint(value)))
}
