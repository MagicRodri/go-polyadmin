package fiber

import (
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
