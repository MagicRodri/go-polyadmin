package core

import (
	"encoding/csv"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// Exporter turns a ModelAdmin's list query result into a downloadable
// file. The adapter feeds it the *same* filtered/ordered result set
// ExecuteListQuery produced for the list view, so an export always
// represents exactly the dataset the user was looking at, respecting
// the resource's configured columns (ListDisplay). Write adds one row
// at a time to w -- for CSVExporter that means the caller can stream
// output as it's produced without buffering the whole dataset in
// memory; XLSXExporter's RowWriter buffers internally instead, since
// the zip container format has to be finalized before any bytes can
// go out (see XLSXRowWriter).
type Exporter interface {
	Format() string
	ContentType() string
	FileExtension() string
	Write(w RowWriter, admin *Admin, modelAdmin ModelAdmin, objects []any, columns []string) error
}

// RowWriter is the sink an Exporter writes rows to -- an io.Writer
// would be format-specific plumbing the exporter shouldn't need to
// know about, so this is the narrower interface CSV (and any future
// row-oriented format) actually needs.
type RowWriter interface {
	Write(row []string) error
	Flush()
}

// CellValue is a field's export-friendly value: relation fields
// resolve to their target's display label (or a comma-joined list of
// them for ManyToMany) rather than exporting a raw Go value.
func CellValue(admin *Admin, field Field, obj any) string {
	value := field.GetValue(obj)
	switch field.Type {
	case FieldTypeForeignKey, FieldTypeOneToOne:
		if isNilAny(value) {
			return ""
		}
		targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
		if !ok {
			return stringify(value)
		}
		displayField, _ := targetAdmin.Field(field.Relation.DisplayField)
		return stringify(displayField.GetValue(value))
	case FieldTypeManyToMany:
		items, _ := value.([]any)
		if len(items) == 0 {
			return ""
		}
		targetAdmin, ok := admin.GetModelAdmin(field.Relation.Target)
		if !ok {
			return stringify(value)
		}
		displayField, _ := targetAdmin.Field(field.Relation.DisplayField)
		out := ""
		for i, item := range items {
			if i > 0 {
				out += ", "
			}
			out += stringify(displayField.GetValue(item))
		}
		return out
	default:
		if value == nil {
			return ""
		}
		return stringify(value)
	}
}

// writeRows is the shared header+row loop both CSVExporter and
// XLSXExporter use -- they differ only in what RowWriter they're
// handed and how that writer eventually turns into bytes, not in how
// rows get built.
func writeRows(w RowWriter, admin *Admin, modelAdmin ModelAdmin, objects []any, columns []string, format string) error {
	fields := make([]Field, len(columns))
	header := make([]string, len(columns))
	for i, name := range columns {
		field, _ := modelAdmin.Field(name)
		fields[i] = field
		header[i] = field.Label
	}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("polyadmin: writing %s header: %w", format, err)
	}
	for _, obj := range objects {
		row := make([]string, len(fields))
		for i, field := range fields {
			row[i] = CellValue(admin, field, obj)
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("polyadmin: writing %s row: %w", format, err)
		}
	}
	w.Flush()
	return nil
}

type CSVExporter struct{}

func (CSVExporter) Format() string        { return "csv" }
func (CSVExporter) ContentType() string   { return "text/csv" }
func (CSVExporter) FileExtension() string { return "csv" }

func (CSVExporter) Write(w RowWriter, admin *Admin, modelAdmin ModelAdmin, objects []any, columns []string) error {
	return writeRows(w, admin, modelAdmin, objects, columns, "csv")
}

// csvRowWriter adapts encoding/csv.Writer to RowWriter.
type csvRowWriter struct {
	writer *csv.Writer
}

func NewCSVRowWriter(writer *csv.Writer) RowWriter {
	return csvRowWriter{writer: writer}
}

func (w csvRowWriter) Write(row []string) error { return w.writer.Write(row) }
func (w csvRowWriter) Flush()                   { w.writer.Flush() }

// XLSXExporter writes rows into a real .xlsx workbook via excelize.
// Every cell is written as a string -- CellValue already stringifies
// everything for CSV, and reusing that here keeps the two formats'
// output consistent instead of re-deriving typed (int/float/bool)
// cells the way Python's XLSXExporter does.
type XLSXExporter struct{}

func (XLSXExporter) Format() string { return "xlsx" }
func (XLSXExporter) ContentType() string {
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}
func (XLSXExporter) FileExtension() string { return "xlsx" }

func (XLSXExporter) Write(w RowWriter, admin *Admin, modelAdmin ModelAdmin, objects []any, columns []string) error {
	return writeRows(w, admin, modelAdmin, objects, columns, "xlsx")
}

// XLSXRowWriter adapts an excelize.File to RowWriter. Unlike CSV,
// XLSX's zip container can't be streamed row-by-row to an HTTP
// response -- it has to be fully built in memory and finalized once,
// so Write accumulates rows and Flush is a no-op; call Bytes after
// the Exporter's Write returns to get the finished file.
type XLSXRowWriter struct {
	file    *excelize.File
	sheet   string
	nextRow int
}

// NewXLSXRowWriter creates a single-sheet workbook named after the
// resource being exported (matching the Python adapter's
// workbook.create_sheet(model_admin.get_verbose_name())).
func NewXLSXRowWriter(sheetName string) *XLSXRowWriter {
	file := excelize.NewFile()
	file.SetSheetName(file.GetSheetName(0), sheetName)
	return &XLSXRowWriter{file: file, sheet: sheetName, nextRow: 1}
}

func (w *XLSXRowWriter) Write(row []string) error {
	for i, value := range row {
		cell, err := excelize.CoordinatesToCellName(i+1, w.nextRow)
		if err != nil {
			return err
		}
		if err := w.file.SetCellValue(w.sheet, cell, value); err != nil {
			return err
		}
	}
	w.nextRow++
	return nil
}

func (w *XLSXRowWriter) Flush() {}

// Bytes serializes the finished workbook. Only valid after the
// Exporter's Write has returned -- see the XLSXRowWriter doc comment.
func (w *XLSXRowWriter) Bytes() ([]byte, error) {
	buf, err := w.file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("polyadmin: serializing xlsx: %w", err)
	}
	return buf.Bytes(), nil
}
