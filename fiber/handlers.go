package fiber

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

var filterKeyPattern = regexp.MustCompile(`^filter\[(\w+)\]$`)

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
	pageSize, err := strconv.Atoi(c.Query("page_size", "25"))
	if err != nil {
		pageSize = 25
	}
	return core.ListRequest{
		Search:   c.Query("search"),
		Filters:  filters,
		Ordering: c.Query("sort"),
		Page:     page,
		PageSize: pageSize,
	}
}

func isHTMXRequest(c *fiber.Ctx) bool {
	return c.Get("HX-Request") == "true"
}

func writeAuthError(c *fiber.Ctx, result authResult) error {
	if result == authUnauthenticated {
		return c.Status(fiber.StatusUnauthorized).SendString("Authentication required.")
	}
	return c.Status(fiber.StatusForbidden).SendString("Permission denied.")
}

func queryset(ctx context.Context, modelAdmin core.ModelAdmin) ([]any, error) {
	value, err := modelAdmin.GetQueryset(ctx)
	if err != nil {
		return nil, err
	}
	items, _ := value.([]any)
	return items, nil
}

func handleList(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "list"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		perms := computePermissions(admin, principal, modelAdmin)
		relPerms := computeRelationPermissions(admin, principal, modelAdmin, modelAdmin.ListDisplay())

		req := parseListRequest(c)
		objects, err := queryset(c.Context(), modelAdmin)
		if err != nil {
			return err
		}
		objects = core.ExecuteListQuery(modelAdmin, objects, req)
		page := core.Paginate(objects, req.Page, req.PageSize)

		var html string
		if isHTMXRequest(c) {
			html, err = renderer.RenderListFragment(principal, modelAdmin, page, req, perms, relPerms)
		} else {
			html, err = renderer.RenderList(principal, modelAdmin, page, req, perms, relPerms, popFlash(c))
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

func handleDetail(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		perms := computePermissions(admin, principal, modelAdmin)
		relPerms := computeRelationPermissions(admin, principal, modelAdmin, modelAdmin.DetailFields())
		html, err := renderer.RenderDetail(principal, modelAdmin, obj, perms, relPerms, popFlash(c))
		if err != nil {
			return err
		}
		clearFlash(c)
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func parseFormData(c *fiber.Ctx, modelAdmin core.ModelAdmin) map[string]any {
	data := make(map[string]any, len(modelAdmin.FormFields()))
	for _, name := range modelAdmin.FormFields() {
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
			data[name] = field.ParseFormValue(c.FormValue(name))
		}
	}
	return data
}

func handleCreateGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "create"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		relOptions := computeRelationOptions(admin, modelAdmin, nil)
		html, err := renderer.RenderForm(principal, modelAdmin, nil, nil, nil, relOptions)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleCreatePost(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "create"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		data := parseFormData(c, modelAdmin)
		errs := modelAdmin.Validate(data)
		if len(errs) > 0 {
			relOptions := computeRelationOptions(admin, modelAdmin, nil)
			var html string
			var err error
			if isHTMXRequest(c) {
				html, err = renderer.RenderFormFragment(principal, modelAdmin, nil, data, errs, relOptions)
			} else {
				html, err = renderer.RenderForm(principal, modelAdmin, nil, data, errs, relOptions)
			}
			if err != nil {
				return err
			}
			c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
			return c.Status(fiber.StatusUnprocessableEntity).SendString(html)
		}
		obj, err := modelAdmin.Create(c.Context(), data)
		if err != nil {
			return err
		}
		setFlash(c, "success", modelAdmin.VerboseName()+" created.")
		target := basePath + "/" + slug + "/" + stringOrEmpty(modelAdmin.GetPK(obj))
		if c.FormValue("_continue") != "" {
			target += "/edit"
		}
		return redirectTo(c, target)
	}
}

func handleEditGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		relOptions := computeRelationOptions(admin, modelAdmin, obj)
		html, err := renderer.RenderForm(principal, modelAdmin, obj, nil, nil, relOptions)
		if err != nil {
			return err
		}
		c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
		return c.SendString(html)
	}
}

func handleEditPost(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		data := parseFormData(c, modelAdmin)
		errs := modelAdmin.Validate(data)
		if len(errs) > 0 {
			relOptions := computeRelationOptions(admin, modelAdmin, obj)
			var html string
			if isHTMXRequest(c) {
				html, err = renderer.RenderFormFragment(principal, modelAdmin, obj, data, errs, relOptions)
			} else {
				html, err = renderer.RenderForm(principal, modelAdmin, obj, data, errs, relOptions)
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
		setFlash(c, "success", modelAdmin.VerboseName()+" updated.")
		target := basePath + "/" + slug + "/" + c.Params("pk")
		if c.FormValue("_continue") != "" {
			target += "/edit"
		}
		return redirectTo(c, target)
	}
}

func handleDeleteGet(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(obj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		html, err := renderer.RenderDelete(principal, modelAdmin, obj)
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
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if !core.IsNil(obj) {
			if err := modelAdmin.Delete(c.Context(), obj); err != nil {
				return err
			}
			setFlash(c, "success", modelAdmin.VerboseName()+" deleted.")
		}
		return redirectTo(c, basePath+"/"+slug)
	}
}

// handleDeleteHTMX is the list view's row-level Delete button: removes
// just that row (empty response; hx-swap="outerHTML" on the <tr> makes
// it vanish) instead of redirecting anywhere.
func handleDeleteHTMX(admin *core.Admin, modelAdmin core.ModelAdmin) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "delete"), modelAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		obj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if !core.IsNil(obj) {
			if err := modelAdmin.Delete(c.Context(), obj); err != nil {
				return err
			}
		}
		return c.SendString("")
	}
}

// handleLookup serves GET /{slug}/lookup?q=... -- an HTML fragment of
// matching options for this resource, meant to be consumed
// by another resource's autocomplete combobox (the ui/field.html
// template's combobox branch, see render_helpers.go's formInputHTML).
// Gated on *this* resource's own "view" permission, since that's what's
// actually being browsed.
func handleLookup(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		if _, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		query := c.Query("q")
		displayName := c.Query("display")
		if displayName == "" && len(modelAdmin.SearchFields()) > 0 {
			displayName = modelAdmin.SearchFields()[0]
		}
		var displayField core.Field
		var hasDisplayField bool
		if displayName != "" {
			displayField, hasDisplayField = modelAdmin.Field(displayName)
		}

		req := core.ListRequest{Search: query}
		objects, err := queryset(c.Context(), modelAdmin)
		if err != nil {
			return err
		}
		objects = core.ExecuteListQuery(modelAdmin, objects, req)
		if len(objects) > 20 {
			objects = objects[:20]
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

// handleAction serves POST /{slug}/actions/:name -- runs a ModelAdmin
// Action over the objects named by the "pks" form field.
// Serves both entry points with the same route: the list view's
// bulk-select form posts every checked row's pk, the detail page's
// per-record action buttons post a single-item "pks". Mirrors the
// FastAPI adapter's build_action_handler, including the flash message
// left for whichever page (list or detail) the redirect lands on.
func handleAction(admin *core.Admin, modelAdmin core.ModelAdmin, basePath string) fiber.Handler {
	slug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		principal, result := authorize(admin, c, core.ResourcePermission(slug, "view"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		action, ok := core.GetAction(modelAdmin, c.Params("name"))
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		if action.Permission != "" && admin.Authorizer != nil {
			if !admin.Authorizer.Can(principal, core.ResourcePermission(slug, action.Permission), modelAdmin) {
				return c.Status(fiber.StatusForbidden).SendString("Permission denied.")
			}
		}

		raw := c.Context().PostArgs().PeekMulti("pks")
		redirectTarget := c.Get("Referer")
		if redirectTarget == "" {
			redirectTarget = basePath + "/" + slug
		}
		if len(raw) == 0 {
			setFlash(c, "warning", "No items selected.")
			return redirectTo(c, redirectTarget)
		}

		objects := make([]any, 0, len(raw))
		for _, pk := range raw {
			obj, err := modelAdmin.GetObject(c.Context(), string(pk))
			if err != nil {
				return err
			}
			if !core.IsNil(obj) {
				objects = append(objects, obj)
			}
		}
		message, err := action.Handler(c.Context(), modelAdmin, objects, principal)
		if err != nil {
			return err
		}
		if message == "" {
			message = fmt.Sprintf("%s applied to %d record(s).", action.Label, len(objects))
		}
		setFlash(c, "success", message)
		return redirectTo(c, redirectTarget)
	}
}

// handleInlineCreate serves POST {slug}/{pk}/inlines/:child -- creates
// one inline child row (see core/inline.go, docs/inlines.md). The
// response always carries just the freshly rebuilt whole inline
// section (full-region-swap, matching RenderListFragment/
// RenderFormFragment's existing idiom), never a redirect and never
// the whole parent page.
func handleInlineCreate(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "create"), childAdmin); result != authOK {
			return writeAuthError(c, result)
		}

		data := parseFormData(c, childAdmin)
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
func handleInlineUpdate(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "update"), childAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		childPK := c.Params("childPK")
		childObj, err := childAdmin.GetObject(c.Context(), childPK)
		if err != nil {
			return err
		}
		if core.IsNil(childObj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}

		data := parseFormData(c, childAdmin)
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
func handleInlineDelete(admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer, basePath string) fiber.Handler {
	parentSlug := modelAdmin.Slug()
	return func(c *fiber.Ctx) error {
		inline, ok := findInline(modelAdmin, c.Params("child"))
		if !ok {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		principal, result := authorize(admin, c, core.ResourcePermission(parentSlug, "update"), modelAdmin)
		if result != authOK {
			return writeAuthError(c, result)
		}
		parentObj, err := modelAdmin.GetObject(c.Context(), c.Params("pk"))
		if err != nil {
			return err
		}
		if core.IsNil(parentObj) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		childAdmin, _ := admin.GetModelAdmin(inline.Child)
		if _, result := authorize(admin, c, core.ResourcePermission(inline.Child, "delete"), childAdmin); result != authOK {
			return writeAuthError(c, result)
		}
		childObj, err := childAdmin.GetObject(c.Context(), c.Params("childPK"))
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

// redirectTo mirrors the FastAPI adapter's HTMX-aware redirect: a
// plain 303 for a normal browser navigation, an HX-Redirect response
// for an HTMX request, since htmx otherwise treats a redirected AJAX
// response as content to swap in, not a page navigation.
func redirectTo(c *fiber.Ctx, url string) error {
	if isHTMXRequest(c) {
		c.Set("HX-Redirect", url)
		return c.SendString("")
	}
	return c.Redirect(url, fiber.StatusSeeOther)
}
