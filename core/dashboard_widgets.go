package core

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type Tile struct {
	Label string
	Value any
	Icon  string
	Hint  string
}

// MetricGroup is a row of headline numbers produced by one data call --
// one request to a service that answers all of them, where separate
// Metrics would make one each.
type MetricGroup struct {
	baseWidget
	GetTiles func(ctx context.Context, wc WidgetContext) ([]Tile, error)
}

func NewMetricGroup(title string, getTiles func(ctx context.Context, wc WidgetContext) ([]Tile, error), opts ...WidgetOption) MetricGroup {
	return MetricGroup{baseWidget: newBaseWidget(title, "admin/widgets/metric_group.html", opts), GetTiles: getTiles}
}

func (m MetricGroup) GetData() any { return nil }

func (m MetricGroup) Data(ctx context.Context, wc WidgetContext) (any, error) {
	tiles, err := m.GetTiles(ctx, wc)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Tiles": tiles, "Empty": len(tiles) == 0}, nil
}

var ColumnFormats = []string{"text", "number", "datetime", "share", "percent"}

type Column struct {
	Key    string
	Label  string
	Align  string // "start" (default) or "end"
	Format string // one of ColumnFormats; "" is "text"
	Strong bool
	Empty  string // shown as a muted badge for an empty cell
	// Tones colour a numeric value: it renders as a badge in the variant of
	// the highest Min it reaches, e.g. 80 success, 50 warning, 0 danger.
	Tones []Tone
}

type Tone struct {
	Min     float64
	Variant string // "success", "warning" or "danger"
}

var ToneVariants = []string{"success", "warning", "danger"}

func (c Column) validate() error {
	if c.Format != "" && !slices.Contains(ColumnFormats, c.Format) {
		return fmt.Errorf("column %q: format must be one of %v, not %q", c.Key, ColumnFormats, c.Format)
	}
	for _, tone := range c.Tones {
		if !slices.Contains(ToneVariants, tone.Variant) {
			return fmt.Errorf("column %q: tone must be one of %v, not %q", c.Key, ToneVariants, tone.Variant)
		}
	}
	if c.Align != "" && c.Align != "start" && c.Align != "end" {
		return fmt.Errorf("column %q: align must be start or end, not %q", c.Key, c.Align)
	}
	return nil
}

// Rows is one page of a DataTable. Total is the size of the whole result
// when known; Totals is a summary row shown above the first page.
type Rows struct {
	Items  []map[string]any
	Total  *int
	Totals map[string]any
}

func Total(n int) *int { return &n }

// NextOffset is where the next page starts; ok is false when this was the
// last one. With no Total, a page shorter than the limit is the last.
func NextOffset(rows Rows, offset, limit int) (int, bool) {
	if limit <= 0 || len(rows.Items) == 0 {
		return 0, false
	}
	shown := offset + len(rows.Items)
	more := len(rows.Items) == limit
	if rows.Total != nil {
		more = shown < *rows.Total
	}
	if !more {
		return 0, false
	}
	return shown, true
}

type DataTablePage struct {
	Columns    []Column
	Rows       Rows
	NextOffset int
	HasNext    bool
	Empty      bool
	Footer     string
}

// DataTable is rows the host fetches page by page, e.g. from another
// service. Scrolling to the last row loads the next page.
type DataTable struct {
	baseWidget
	Columns           []Column
	GetRows           func(ctx context.Context, wc WidgetContext) (Rows, error)
	PageSizeValue     int // 0 = one load, no paging
	Searchable        bool
	SearchPlaceholder string
	TotalLabel        string // "{total}" is replaced by Rows.Total
}

func NewDataTable(title string, columns []Column, getRows func(ctx context.Context, wc WidgetContext) (Rows, error), opts ...WidgetOption) *DataTable {
	return &DataTable{baseWidget: newBaseWidget(title, "admin/widgets/data_table.html", opts), Columns: columns, GetRows: getRows, PageSizeValue: 50}
}

func (t *DataTable) PageSize() int { return t.PageSizeValue }
func (t *DataTable) GetData() any  { return nil }

func (t *DataTable) validate() error {
	for _, c := range t.Columns {
		if err := c.validate(); err != nil {
			return fmt.Errorf("dashboard widget %q: %w", WidgetKey(t), err)
		}
	}
	return nil
}

func (t *DataTable) Data(ctx context.Context, wc WidgetContext) (any, error) {
	rows, err := t.GetRows(ctx, wc)
	if err != nil {
		return nil, err
	}
	next, more := NextOffset(rows, wc.Offset, wc.Limit)
	page := DataTablePage{
		Columns: t.Columns, Rows: rows, NextOffset: next, HasNext: more,
		Empty: wc.Offset == 0 && len(rows.Items) == 0 && len(rows.Totals) == 0,
	}
	if t.TotalLabel != "" && rows.Total != nil {
		page.Footer = strings.ReplaceAll(T(ctx, t.TotalLabel), "{total}", strconv.Itoa(*rows.Total))
	}
	return page, nil
}
