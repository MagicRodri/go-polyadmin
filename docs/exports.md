# Exports

`GET /{slug}/export/{format}` streams the resource's `DisplayFields`
columns as a downloadable file, gated by its own `.export` permission
(independent of `.view` — a principal can be allowed to browse a
resource without being allowed to export it, or vice versa).

## It's the same data you're looking at

The export route runs the exact same query pipeline the list view
does — search, every active filter, and ordering, all read from the
request's query string — before handing the result to the exporter.
Concretely: the "Export CSV" link rendered on the list page carries
the current search/filter/sort along in its own query string, so
exporting after searching/filtering exports what's on screen, not the
whole table. There's no separate export-specific query API to keep in
sync with the list view's.

## Formats

| Format | Exporter |
|---|---|
| CSV | `core.CSVExporter{}` |
| XLSX | `core.XLSXExporter{}`, via [excelize](https://github.com/xuri/excelize) |

CSV is genuinely streamed row-by-row to the HTTP response — the
exporter never holds more than one row in memory, so export size is
bounded by how long you're willing to keep the connection open, not by
RAM. XLSX is a partial exception: the zip container format has to be
finalized before any bytes go out, so unlike CSV it's buffered once
before sending, not chunked to the client as it's generated
(`core.XLSXRowWriter` builds the whole workbook in memory via excelize
before serializing it). Keep XLSX exports to a reasonable row count for
that reason; CSV doesn't have this limit.

XLSX cells are all strings, since `core.CellValue` already stringifies
everything for CSV and `XLSXExporter` reuses it rather than re-deriving
typed cells.

## Cell values

A plain field exports its value as-is. A foreign-key/one-to-one field
exports its *target's* display label (whatever `Relation.DisplayField`
points at), not a raw object repr; a many-to-many field exports a
comma-joined list of its related objects' display labels. This mirrors
how the same relation renders in the list/detail views, so an export
column and its on-screen equivalent always agree.

## Adding a custom format

`core.Exporter` is an interface — `Format()`, `ContentType()`,
`FileExtension()`, `Write(w RowWriter, ...)` — so a custom format is
just another implementation. Route registration in `Mount` wires in CSV
and XLSX only, so a third format isn't picked up automatically; add
your own route by hand outside `Mount`.
