package fiber

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

var filterKeyPattern = regexp.MustCompile(`^filter\[(\w+)\]$`)

// selectAllField is the hidden flag the bulk-actions form sets when the
// user chose "select all N matching" rather than ticking rows.
const selectAllField = "_select_all"

// The submit buttons meaning something other than "save and show me the
// record". An unclicked submit button's name never reaches the server, so
// the handler reads presence rather than a value.
const (
	saveContinueField   = "_continue"
	saveAddAnotherField = "_addanother"
)

// parseListRequestFromForm rebuilds the list query from the posted form,
// not the URL: a bulk action posts to its own route, so reading the query
// string would silently act on the unfiltered set.
func parseListRequestFromForm(c *fiber.Ctx) core.ListRequest {
	filters := make(map[string]string)
	c.Context().PostArgs().VisitAll(func(key, value []byte) {
		if match := filterKeyPattern.FindStringSubmatch(string(key)); match != nil {
			filters[match[1]] = string(value)
		}
	})
	return core.ListRequest{
		Search:   formValue(c, "search"),
		Filters:  filters,
		Ordering: formValue(c, "sort"),
	}
}

func parseListRequest(c *fiber.Ctx) core.ListRequest {
	filters := make(map[string]string)
	c.Context().QueryArgs().VisitAll(func(key, value []byte) {
		if match := filterKeyPattern.FindStringSubmatch(string(key)); match != nil {
			filters[match[1]] = string(value)
		}
	})
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil {
		page = 1
	}
	// No default here: an unset page_size stays 0 so ApplyDefaults can fall
	// back to the ModelAdmin's own PageSize() first. Defaulting here would
	// make that unreachable.
	pageSize, err := strconv.Atoi(c.Query("page_size"))
	if err != nil {
		pageSize = 0
	}
	return core.ListRequest{
		Search:   queryValue(c, "search"),
		Filters:  filters,
		Ordering: queryValue(c, "sort"),
		Page:     page,
		PageSize: pageSize,
	}
}

func isHTMXRequest(c *fiber.Ctx) bool {
	return c.Get("HX-Request") == "true"
}

// writeAuthError turns a rejected authorize() into a response.
//
// Which answer the unauthenticated case gets depends on whether there is a
// login page to offer: with a LoginBackend the browser is redirected there
// carrying where it was going, without one 401 is the whole story.
// Forbidden never redirects -- the visitor is signed in and simply may not
// do this, so a login form would invite them to re-authenticate as the
// same person to the same refusal.
func writeAuthError(c *fiber.Ctx, admin *core.Admin, basePath string, result authResult) error {
	if result != authUnauthenticated {
		return writeForbidden(c, admin, basePath)
	}
	if admin.LoginBackend == nil {
		return writeUnauthenticated(c, admin, basePath)
	}
	// redirectTo, not c.Redirect: an expired session usually surfaces mid-
	// page on an htmx request, where a 303 would be swapped in as content.
	return redirectTo(c, loginURL(basePath, requestedURL(c)))
}

func queryset(ctx context.Context, modelAdmin core.ModelAdmin) ([]any, error) {
	value, err := modelAdmin.GetQueryset(ctx)
	if err != nil {
		return nil, err
	}
	items, _ := value.([]any)
	return items, nil
}

func handleList(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "list"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		// nil: a list page is about the model, not one record.
		perms := computePermissions(admin, principal, modelAdmin, nil)
		relPerms := computeRelationPermissions(admin, principal, modelAdmin, modelAdmin.ListDisplay())

		// Resolved once, then handed to both the query and the pager --
		// otherwise PageOf would size the page control from the raw
		// request and disagree with the rows actually fetched.
		req := core.ApplyDefaults(modelAdmin, parseListRequest(c))
		objects, total, err := core.ListObjects(c.Context(), modelAdmin, req)
		if err != nil {
			return err
		}
		page := core.PageOf(objects, total, req)

		var html string
		if isHTMXRequest(c) {
			html, err = renderer.RenderListFragment(principal, csrfToken(c), modelAdmin, page, req, perms, relPerms)
		} else {
			html, err = renderer.RenderList(principal, csrfToken(c), modelAdmin, page, req, perms, relPerms, popFlash(c))
		}
		if err != nil {
			return err
		}
		if !isHTMXRequest(c) {
			clearFlash(c)
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleDetail(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return writeNotFound(c, admin, basePath)
		}
		// The record's own page: per-object rules decide whether it
		// offers Edit/Delete at all.
		if !authorizeObject(admin, principal, core.ResourcePermission(slug, "view"), obj) {
			return writeForbidden(c, admin, basePath)
		}
		perms := computePermissions(admin, principal, modelAdmin, obj)
		relPerms := computeRelationPermissions(admin, principal, modelAdmin, modelAdmin.DetailFields())
		html, err := renderer.RenderDetail(c.Context(), principal, csrfToken(c), modelAdmin, obj, perms, relPerms, popFlash(c))
		if err != nil {
			return err
		}
		clearFlash(c)
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

// validateWritable runs the ModelAdmin's validation, then drops complaints
// about read-only fields. Such a field is never posted, so a required one
// would otherwise fail every save: the value is not missing, it is simply
// not the form's to send. Wrapping rather than changing Validate leaves an
// application's own override unaffected.
func validateWritable(modelAdmin core.ModelAdmin, data map[string]any, obj any) map[string][]string {
	errs := modelAdmin.Validate(data)
	for name := range errs {
		if modelAdmin.IsReadOnly(name, obj) {
			delete(errs, name)
		}
	}
	return errs
}

// formValue reads a posted field and returns a string that outlives the
// request.
//
// Not defensive tidiness but a correctness fix: c.FormValue returns a
// string pointing into fasthttp's request buffer, and fasthttp reuses
// that buffer for the next request, so a value stored on a record
// silently becomes whatever the next request posted. Create
// "aaa@example.com" then "bbb@example.com", and the first record's
// email had become "bbb@example.com" with no write to it.
func formValue(c *fiber.Ctx, name string) string {
	return strings.Clone(c.FormValue(name))
}

// pathParam is formValue's counterpart for a URL segment: c.Params has
// the same request-buffer lifetime, and a pk reaches application code
// (GetObject, and whatever it keys off).
func pathParam(c *fiber.Ctx, name string) string {
	return strings.Clone(c.Params(name))
}

// queryValue is formValue's counterpart for the query string, which
// c.Query also returns out of the request buffer. The list request built
// from it reaches a ListQuerier, i.e. application code.
func queryValue(c *fiber.Ctx, name string) string {
	return strings.Clone(c.Query(name))
}

// parseFormData reads the posted form into a data map. obj is the record
// being edited, nil when creating, and is passed only to resolve
// read-only fields: such a field is skipped entirely, so a crafted POST
// naming it cannot write it. Omitting the input is presentation; this is
// the enforcement.
func parseFormData(c *fiber.Ctx, modelAdmin core.ModelAdmin, obj any) map[string]any {
	data := make(map[string]any, len(modelAdmin.FormFields()))
	for _, name := range modelAdmin.FormFields() {
		if modelAdmin.IsReadOnly(name, obj) {
			continue
		}
		field, _ := modelAdmin.Field(name)
		switch field.Type {
		case core.FieldTypeBoolean:
			data[name] = c.FormValue(name) != ""
		case core.FieldTypeManyToMany:
			raw := c.Context().PostArgs().PeekMulti(name)
			values := make([]string, len(raw))
			for i, v := range raw {
				values[i] = string(v)
			}
			data[name] = values
		default:
			data[name] = field.ParseFormValue(formValue(c, name))
		}
	}
	return data
}

func handleCreateGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "create"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		relOptions := computeRelationOptions(admin, modelAdmin, nil)
		html, err := renderer.RenderForm(principal, csrfToken(c), modelAdmin, nil, nil, nil, relOptions)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleCreatePost(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "create"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		data := parseFormData(c, modelAdmin, nil)
		errs := validateWritable(modelAdmin, data, nil)
		if len(errs) > 0 {
			relOptions := computeRelationOptions(admin, modelAdmin, nil)
			var html string
			var err error
			if isHTMXRequest(c) {
				html, err = renderer.RenderFormFragment(principal, csrfToken(c), modelAdmin, nil, data, errs, relOptions)
			} else {
				html, err = renderer.RenderForm(principal, csrfToken(c), modelAdmin, nil, data, errs, relOptions)
			}
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Status(fiber.StatusUnprocessableEntity).SendString(html)
		}
		obj, err := modelAdmin.Create(c.Context(), data)
		if err == nil {
			recordAudit(c.Context(), admin, principal, modelAdmin, core.AuditCreate, obj)
		}
		if err != nil {
			return err
		}
		setFlash(c, "success", modelAdmin.VerboseName()+" created.")
		// "Save and add another" goes back to an empty form, which is the
		// whole point when entering records in a batch -- checked before
		// building the record's own URL, since it never uses one.
		if c.FormValue(saveAddAnotherField) != "" {
			return redirectTo(c, basePath+"/"+slug+"/create")
		}
		target := basePath + "/" + slug + "/" + stringOrEmpty(modelAdmin.GetPK(obj))
		if c.FormValue(saveContinueField) != "" {
			target += "/edit"
		}
		return redirectTo(c, target)
	}
}

func handleEditGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return writeNotFound(c, admin, basePath)
		}
		if !authorizeObject(admin, principal, core.ResourcePermission(slug, "update"), obj) {
			return writeForbidden(c, admin, basePath)
		}
		relOptions := computeRelationOptions(admin, modelAdmin, obj)
		html, err := renderer.RenderForm(principal, csrfToken(c), modelAdmin, obj, nil, nil, relOptions)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleEditPost(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return writeNotFound(c, admin, basePath)
		}
		if !authorizeObject(admin, principal, core.ResourcePermission(slug, "update"), obj) {
			return writeForbidden(c, admin, basePath)
		}
		data := parseFormData(c, modelAdmin, obj)
		errs := validateWritable(modelAdmin, data, obj)
		if len(errs) > 0 {
			relOptions := computeRelationOptions(admin, modelAdmin, obj)
			var html string
			if isHTMXRequest(c) {
				html, err = renderer.RenderFormFragment(principal, csrfToken(c), modelAdmin, obj, data, errs, relOptions)
			} else {
				html, err = renderer.RenderForm(principal, csrfToken(c), modelAdmin, obj, data, errs, relOptions)
			}
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Status(fiber.StatusUnprocessableEntity).SendString(html)
		}
		if _, err := modelAdmin.Update(c.Context(), obj, data); err != nil {
			return err
		}
		recordAudit(c.Context(), admin, principal, modelAdmin, core.AuditUpdate, obj)
		setFlash(c, "success", modelAdmin.VerboseName()+" updated.")
		if c.FormValue(saveAddAnotherField) != "" {
			return redirectTo(c, basePath+"/"+slug+"/create")
		}
		target := basePath + "/" + slug + "/" + c.Params("pk")
		if c.FormValue(saveContinueField) != "" {
			target += "/edit"
		}
		return redirectTo(c, target)
	}
}

func handleDeleteGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return writeNotFound(c, admin, basePath)
		}
		if !authorizeObject(admin, principal, core.ResourcePermission(slug, "delete"), obj) {
			return writeForbidden(c, admin, basePath)
		}
		html, err := renderer.RenderDelete(principal, csrfToken(c), modelAdmin, obj)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleDeletePost(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if !core.IsNil(obj) {
			if !authorizeObject(admin, principal, core.ResourcePermission(slug, "delete"), obj) {
				return writeForbidden(c, admin, basePath)
			}
			if err := modelAdmin.Delete(c.Context(), obj); err != nil {
				return err
			}
			recordAudit(c.Context(), admin, principal, modelAdmin, core.AuditDelete, obj)
			setFlash(c, "success", modelAdmin.VerboseName()+" deleted.")
		}
		return redirectTo(c, basePath+"/"+slug)
	}
}

// handleDeleteHTMX is the list view's row-level Delete button: removes
// just that row (empty response; hx-swap="outerHTML" on the <tr> makes
// it vanish) instead of redirecting anywhere.
func handleDeleteHTMX(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if !core.IsNil(obj) {
			if !authorizeObject(admin, principal, core.ResourcePermission(slug, "delete"), obj) {
				return writeForbidden(c, admin, basePath)
			}
			if err := modelAdmin.Delete(c.Context(), obj); err != nil {
				return err
			}
			recordAudit(c.Context(), admin, principal, modelAdmin, core.AuditDelete, obj)
		}
		return c.SendString("")
	}
}

// lookupLimit caps the autocomplete's suggestions. The control is a
// search box, not a browser -- past a screenful the answer is "type
// more", not "scroll".
const lookupLimit = 20

// handleLookup serves GET /{slug}/lookup?q=..., an HTML fragment of
// matching options consumed by another resource's autocomplete combobox.
// Gated on this resource's own "view" permission, since that is what is
// being browsed.
func handleLookup(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin); result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		query := queryValue(c, "q")
		displayName := queryValue(c, "display")
		if displayName == "" && len(modelAdmin.SearchFields()) > 0 {
			displayName = modelAdmin.SearchFields()[0]
		}
		var displayField core.Field
		var hasDisplayField bool
		if displayName != "" {
			displayField, hasDisplayField = modelAdmin.Field(displayName)
		}

		// The cap rides in as the page window rather than a slice
		// afterwards, so a ListQuerier applies it in its own query
		// instead of returning the whole table for us to trim.
		req := core.ListRequest{Search: query, Page: 1, PageSize: lookupLimit}
		objects, _, err := core.ListObjects(c.Context(), modelAdmin, req)
		if err != nil {
			return err
		}
		options := make([]lookupOption, 0, len(objects))
		for _, obj := range objects {
			label := fmt.Sprint(modelAdmin.GetPK(obj))
			if hasDisplayField {
				label = fmtValue(displayField.GetValue(obj))
			}
			options = append(options, lookupOption{PK: modelAdmin.GetPK(obj), Label: label})
		}
		html, err := renderer.RenderLookup(options)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

// handleAction serves POST /{slug}/actions/:name, running an Action over
// the objects named by the "pks" form field. One route for both entry
// points: the list's bulk-select form posts every checked row, a detail
// page's action button posts a single-item "pks".
func handleAction(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		action, ok := core.GetAction(modelAdmin, c.Params("name"))
		if !ok {
			return writeNotFound(c, admin, basePath)
		}
		if action.Permission != "" && admin.Authorizer != nil {
			if !admin.Authorizer.Can(principal, core.ResourcePermission(slug, action.Permission), modelAdmin) {
				return writeForbidden(c, admin, basePath)
			}
		}

		raw := c.Context().PostArgs().PeekMulti("pks")
		// The Referer is attacker-controlled, so it is validated before
		// being used as a redirect target -- see core.SafeRedirectPath.
		redirectTarget := core.SafeRedirectPath(
			c.Get("Referer"), string(c.Request().Host()), basePath, basePath+"/"+slug)

		// "Select all N matching" posts the filters instead of the pks: a
		// checkbox only reaches the rows on screen. The set is resolved server-
		// side from the same query the list was showing.
		selectAll := c.FormValue(selectAllField) != ""
		if !selectAll && len(raw) == 0 {
			setFlash(c, "warning", "No items selected.")
			return redirectTo(c, redirectTarget)
		}

		var objects []any
		if selectAll {
			req := parseListRequestFromForm(c)
			req.Unlimited = true
			matching, _, err := core.ListObjects(c.Context(), modelAdmin, req)
			if err != nil {
				return err
			}
			objects = matching
		} else {
			objects = make([]any, 0, len(raw))
			for _, pk := range raw {
				obj, err := modelAdmin.GetObject(c.Context(), string(pk))
				if err != nil {
					return err
				}
				if !core.IsNil(obj) {
					objects = append(objects, obj)
				}
			}
		}
		if len(objects) == 0 {
			setFlash(c, "warning", "No items selected.")
			return redirectTo(c, redirectTarget)
		}
		message, err := action.Handler(c.Context(), modelAdmin, objects, principal)
		if err != nil {
			return err
		}
		// One entry per record, not one per action: the log's question
		// is "what happened to this record", and a bulk run over 500
		// rows is 500 answers to it.
		for _, obj := range objects {
			recordAudit(c.Context(), admin, principal, modelAdmin, action.Name, obj)
		}
		if message == "" {
			message = fmt.Sprintf("%s applied to %d record(s).", action.Label, len(objects))
		}
		setFlash(c, "success", message)
		return redirectTo(c, redirectTarget)
	}
}

// handleInlineCreate serves POST {slug}/{pk}/inlines/:child. The response
// is the rebuilt inline section alone -- never a redirect, never the whole
// parent page -- matching the other fragment routes.
func handleInlineCreate(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return writeNotFound(c, admin, basePath)
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return writeNotFound(c, admin, basePath)
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "create"), childAdmin); result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}

		data := parseFormData(c, childAdmin, nil)
		data[inline.FKField] = stringOrEmpty(modelAdmin.GetPK(parentObj))
		errs := childAdmin.Validate(data)
		if len(errs) > 0 {
			html, err := renderer.RenderInlineFragment(principal, modelAdmin, parentObj, inline, nil, data, errs)
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Status(fiber.StatusUnprocessableEntity).SendString(html)
		}

		if _, err := childAdmin.Create(c.Context(), data); err != nil {
			return err
		}
		html, err := renderer.RenderInlineFragment(principal, modelAdmin, parentObj, inline, nil, nil, nil)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

// handleInlineUpdate serves POST {slug}/{pk}/inlines/:child/:childPK
// -- updates one existing inline child row.
func handleInlineUpdate(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return writeNotFound(c, admin, basePath)
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return writeNotFound(c, admin, basePath)
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "update"), childAdmin); result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		childPK := pathParam(c, "childPK")
		childObj, err := childAdmin.GetObject(c.Context(), childPK)
		if err != nil {
			return err
		}
		if core.IsNil(childObj) {
			return writeNotFound(c, admin, basePath)
		}

		data := parseFormData(c, childAdmin, nil)
		data[inline.FKField] = stringOrEmpty(modelAdmin.GetPK(parentObj))
		errs := childAdmin.Validate(data)
		if len(errs) > 0 {
			html, err := renderer.RenderInlineFragment(principal, modelAdmin, parentObj, inline, childPK, data, errs)
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Status(fiber.StatusUnprocessableEntity).SendString(html)
		}

		if _, err := childAdmin.Update(c.Context(), childObj, data); err != nil {
			return err
		}
		html, err := renderer.RenderInlineFragment(principal, modelAdmin, parentObj, inline, nil, nil, nil)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

// handleInlineDelete serves DELETE {slug}/{pk}/inlines/:child/:childPK
// -- removes one inline child row.
func handleInlineDelete(admin *core.Admin, modelAdmin core.ModelAdmin, renderers *Renderers, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		renderer := renderers.For(c)
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return writeNotFound(c, admin, basePath)
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), pathParam(c, "pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return writeNotFound(c, admin, basePath)
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "delete"), childAdmin); result != authOK {
			return writeAuthError(c, admin, basePath, result)
		}
		childObj, err := childAdmin.GetObject(c.Context(), pathParam(c, "childPK"))
		if err != nil {
			return err
		}
		if !core.IsNil(childObj) {
			if err := childAdmin.Delete(c.Context(), childObj); err != nil {
				return err
			}
		}

		html, err := renderer.RenderInlineFragment(principal, modelAdmin, parentObj, inline, nil, nil, nil)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

// redirectTo is htmx-aware: a plain 303 for a browser navigation, HX-
// Redirect for an htmx request, which otherwise swaps the redirected
// response in as content.
func redirectTo(c *fiber.Ctx, url string) error {
	if isHTMXRequest(c) {
		c.Set("HX-Redirect", url)
		return c.SendString("")
	}
	return c.Redirect(url, fiber.StatusSeeOther)
}
