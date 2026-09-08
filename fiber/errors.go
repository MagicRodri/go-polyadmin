package fiber

// Error responses.
//
// Every failure the admin can produce used to be a bare text body --
// `c.Status(403).SendString("Permission denied.")` and friends. Someone
// signed into a themed admin who followed a stale link got a white page
// with three words on it, no navigation, and no way back. These render
// the same failures as an actual page.
//
// Deliberately renderer-independent: the page needs the site title and
// the base path and nothing else, so the five handlers that never took
// a *Renderer (delete POST, the htmx delete, actions, both exports) and
// the CSRF middleware can all report errors without threading one
// through.

import (
	"bytes"
	"html/template"
	"net/http"
	"sync"

	"github.com/MagicRodri/go-polyadmin/core"
	coretemplates "github.com/MagicRodri/go-polyadmin/templates"

	"github.com/gofiber/fiber/v2"
)

// Built once: the error page is request-independent apart from the
// status, the message, and two strings off the Admin.
var (
	errorTemplateOnce sync.Once
	errorTemplate     *template.Template
	errorTemplateErr  error
)

func errorTmpl() (*template.Template, error) {
	errorTemplateOnce.Do(func() {
		tmpl := template.New("error.html").Funcs(templateFuncs)
		tmpl, errorTemplateErr = tmpl.ParseFS(coretemplates.FS, "admin/theme.html", "admin/error.html", "admin/components/error_fragment.html")
		if errorTemplateErr != nil {
			return
		}
		errorTemplate, errorTemplateErr = parseComponents(tmpl)
	})
	return errorTemplate, errorTemplateErr
}

type errorData struct {
	SiteTitle string
	Status    int
	Title     string
	Message   string
	// HomeURL is empty when there is no point offering the link: an
	// unauthenticated visitor cannot reach the dashboard either.
	HomeURL string
}

// writeError renders a failure as a page -- or, for an htmx request, as
// just the alert, since a whole document swapped into a table cell is
// nonsense.
func writeError(c *fiber.Ctx, admin *core.Admin, basePath string, status int, title, message string) error {
	siteTitle := admin.SiteTitle
	if siteTitle == "" {
		siteTitle = "PolyAdmin"
	}
	data := errorData{SiteTitle: siteTitle, Status: status, Title: title, Message: message}
	if status != fiber.StatusUnauthorized {
		data.HomeURL = basePath
	}

	name := "error"
	if isHTMXRequest(c) {
		name = "errorFragment"
	}
	tmpl, err := errorTmpl()
	if err != nil {
		// The error page itself failed to build. Fall back to the plain
		// body rather than returning nothing at all -- an ugly 403 beats
		// a blank 500 that hides which failure actually happened.
		return c.Status(status).SendString(message)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return c.Status(status).SendString(message)
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Status(status).SendString(buf.String())
}

// The four failures the admin produces, each with copy that says what
// happened rather than naming the status code twice.
func writeForbidden(c *fiber.Ctx, admin *core.Admin, basePath string) error {
	return writeError(c, admin, basePath, http.StatusForbidden,
		"Permission denied",
		"Your account doesn't have permission to do that.")
}

func writeNotFound(c *fiber.Ctx, admin *core.Admin, basePath string) error {
	return writeError(c, admin, basePath, http.StatusNotFound,
		"Not found",
		"That record doesn't exist, or it was deleted.")
}

func writeUnauthenticated(c *fiber.Ctx, admin *core.Admin, basePath string) error {
	return writeError(c, admin, basePath, http.StatusUnauthorized,
		"Sign-in required",
		"You need to be signed in to view this page.")
}

func writeCSRFFailure(c *fiber.Ctx, admin *core.Admin, basePath string) error {
	return writeError(c, admin, basePath, http.StatusForbidden,
		"Security check failed",
		"This page expired before the form was submitted. Reload and try again.")
}
