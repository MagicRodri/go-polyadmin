package fiber

import (
	"encoding/csv"
	"fmt"
	"io"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// handleExportCSV exports the *same* filtered/ordered dataset the list
// view would show for the given search/filter/sort query params,
// respecting ListDisplay as the column set. Gated on the resource's
// `.export` permission, independent of `.view`.
func handleExportCSV(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "export"), modelAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		// Unlimited: an export of a filtered set is the whole set, not
		// whichever page the user happened to be looking at.
		req := parseListRequest(c)
		req.Unlimited = true
		objects, _, err := core.ListObjects(c.Context(), modelAdmin, req)
		if err != nil {
			return err
		}

		c.Set(fiber.HeaderContentType, "text/csv")
		c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s.csv"`, slug))
		return c.SendStream(csvStream(admin, modelAdmin, objects))
	}
}

// csvStream returns an io.Reader fed by a pipe that writes CSV rows as
// they're produced, so an exported dataset never has to be fully
// buffered in memory before the response starts sending -- the same
// streaming guarantee the Python CSV exporter makes.
func csvStream(admin *core.Admin, modelAdmin core.ModelAdmin, objects []any) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		writer := csv.NewWriter(pw)
		err := (core.CSVExporter{}).Write(core.NewCSVRowWriter(writer), admin, modelAdmin, objects, modelAdmin.ListDisplay())
		pw.CloseWithError(err)
	}()
	return pr
}

// handleExportXLSX mirrors handleExportCSV, gated and scoped the same
// way, but can't offer the same streaming guarantee: XLSX's zip
// container has to be fully built and finalized before any bytes go
// out (see core.XLSXRowWriter), so the whole workbook is buffered in
// memory once before the response is sent -- same tradeoff the Python
// adapter's openpyxl-backed XLSXExporter makes. Keep exports of this
// format to a reasonable row count for that reason.
func handleExportXLSX(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "export"), modelAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		// Unlimited: an export of a filtered set is the whole set, not
		// whichever page the user happened to be looking at.
		req := parseListRequest(c)
		req.Unlimited = true
		objects, _, err := core.ListObjects(c.Context(), modelAdmin, req)
		if err != nil {
			return err
		}

		writer := core.NewXLSXRowWriter(modelAdmin.VerboseName())
		if err := (core.XLSXExporter{}).Write(writer, admin, modelAdmin, objects, modelAdmin.ListDisplay()); err != nil {
			return err
		}
		data, err := writer.Bytes()
		if err != nil {
			return err
		}

		c.Set(fiber.HeaderContentType, (core.XLSXExporter{}).ContentType())
		c.Set(fiber.HeaderContentDisposition, fmt.Sprintf(`attachment; filename="%s.xlsx"`, slug))
		return c.Send(data)
	}
}
