package fiber

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// The delete preview as the templates see it (docs/deletes.md): headings
// translated, records labelled and linked, nothing the principal may not
// view named.

type deleteItemView struct {
	Label string
	URL   string // "" renders the label as text
}

type deleteGroupView struct {
	Heading string // "Users (12)"
	Items   []deleteItemView
	More    int
	Hidden  int
}

type deletePreviewView struct {
	Cascades  []deleteGroupView
	Protected []deleteGroupView
	Denied    string // translated, comma-joined types the principal may not delete
	Blocked   bool
}

func (r *Renderer) deletePreviewView(p core.ResolvedDeletePreview) deletePreviewView {
	view := deletePreviewView{Blocked: p.Blocked}
	for _, g := range p.Cascades {
		view.Cascades = append(view.Cascades, r.deleteGroupView(g))
	}
	for _, g := range p.Protected {
		view.Protected = append(view.Protected, r.deleteGroupView(g))
	}
	if len(p.DeniedTypes) > 0 {
		labels := make([]string, len(p.DeniedTypes))
		for i, label := range p.DeniedTypes {
			labels[i] = r.t(label)
		}
		view.Denied = strings.Join(labels, ", ")
	}
	return view
}

func (r *Renderer) deleteGroupView(g core.ResolvedDeleteGroup) deleteGroupView {
	view := deleteGroupView{Heading: r.t("%s (%d)", r.t(g.Label), g.Total), More: g.More, Hidden: g.Hidden}
	for _, obj := range g.Visible {
		view.Items = append(view.Items, deleteItemView{
			Label: objectLabel(g.ModelAdmin, obj),
			URL:   fmt.Sprintf("%s/%s/%v", r.basePath, g.ModelAdmin.Slug(), g.ModelAdmin.GetPK(obj)),
		})
	}
	for _, text := range g.Texts {
		view.Items = append(view.Items, deleteItemView{Label: text})
	}
	return view
}

// resolveForDelete runs the preview in the request's context. Every
// delete route calls it immediately before deleting: the page showed the
// preview, but the route is the boundary.
func resolveForDelete(c *fiber.Ctx, admin *core.Admin, modelAdmin core.ModelAdmin, principal *core.Principal, objects []any) (core.ResolvedDeletePreview, error) {
	return core.ResolveDeletePreview(c.Context(), admin, modelAdmin, principal, objects)
}

// The delete_selected confirmation form's own fields (docs/deletes.md).
const (
	confirmedField   = "_confirmed"
	fingerprintField = "_fingerprint"
	returnField      = "_return"
)

// deleteSelection is what the confirmation page re-posts: the pks of an
// explicit selection, or the list's query plus the fingerprint of the set
// it resolved to.
type deleteSelection struct {
	Objects     []any
	SelectAll   bool
	PKs         []string
	List        core.ListRequest
	Fingerprint string
	Return      string // where to land afterwards, already validated
	Changed     bool   // the confirmed set is not the reviewed one
}

// confirmDeleteSelected runs before delete_selected on a ModelAdmin that
// previews deletes. It returns done=true when it answered the request
// with the confirmation page (and deleted nothing); done=false means the
// confirmed, unchanged, unblocked selection may be deleted.
func confirmDeleteSelected(c *fiber.Ctx, admin *core.Admin, modelAdmin core.ModelAdmin, renderer *Renderer,
	principal *core.Principal, objects []any, selectAll bool, returnTo string) (bool, error) {
	fingerprint := core.SelectionFingerprint(modelAdmin, objects)
	confirmed := c.FormValue(confirmedField) != ""
	changed := confirmed && selectAll && formValue(c, fingerprintField) != fingerprint
	preview, err := resolveForDelete(c, admin, modelAdmin, principal, objects)
	if err != nil {
		return true, err
	}
	if confirmed && !changed && !preview.Blocked {
		return false, nil
	}
	sel := deleteSelection{Objects: objects, SelectAll: selectAll, Fingerprint: fingerprint, Return: returnTo, Changed: changed}
	if selectAll {
		sel.List = parseListRequestFromForm(c)
	} else {
		for _, obj := range objects {
			sel.PKs = append(sel.PKs, fmt.Sprint(modelAdmin.GetPK(obj)))
		}
	}
	html, err := renderer.RenderDeleteSelected(principal, csrfToken(c), modelAdmin, sel, preview)
	if err != nil {
		return true, err
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return true, c.SendString(html)
}

type deleteSelectedData struct {
	pageBase
	Heading   string
	Items     []deleteItemView
	More      int
	Selection deleteSelection
	Preview   deletePreviewView
}

func (r *Renderer) RenderDeleteSelected(principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, sel deleteSelection, preview core.ResolvedDeletePreview) (string, error) {
	count := len(sel.Objects)
	title := r.tn("Delete %d record?", "Delete %d records?", count, count)
	crumbs := append(r.categoryBreadcrumb(modelAdmin.Category()),
		breadcrumb{Label: r.t(modelAdmin.VerboseName()), URL: fmt.Sprintf("%s/%s", r.basePath, modelAdmin.Slug())},
		breadcrumb{Label: r.t("Delete"), Active: true})
	data := deleteSelectedData{
		pageBase:  r.pageBase(principal, csrfToken, title, title, "resource:"+modelAdmin.Slug(), crumbs, nil),
		Heading:   title,
		Selection: sel,
		Preview:   r.deletePreviewView(preview),
	}
	sample := sel.Objects
	if len(sample) > core.DeletePreviewSample {
		sample = sample[:core.DeletePreviewSample]
	}
	data.More = count - len(sample)
	for _, obj := range sample {
		item := deleteItemView{Label: objectLabel(modelAdmin, obj)}
		if computePermissions(r.admin, principal, modelAdmin, obj).CanView {
			item.URL = fmt.Sprintf("%s/%s/%v", r.basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj))
		}
		data.Items = append(data.Items, item)
	}
	tmpl, err := r.contentTemplate(modelAdmin, "delete_selected", r.deleteSelectedTpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
