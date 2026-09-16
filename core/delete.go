package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// DeletePreviewer is an optional ModelAdmin capability: implement it to
// show what else a delete takes out, and to block deletes your storage
// would refuse (docs/deletes.md). Same optional-capability shape as
// ListQuerier and AuditReader: implement more, get more, and nothing
// breaks if you do not.
//
// objects is always a list -- one record from the delete page, the whole
// selection from delete_selected -- so an implementation over SQL can
// answer a bulk delete with one count and one sample query per relation.
type DeletePreviewer interface {
	DeletePreview(ctx context.Context, objects []any) (DeletePreview, error)
}

// DeletePreview is what deleting objects would take with it.
type DeletePreview struct {
	Cascades  []DeleteGroup // deleted along with the objects
	Protected []DeleteGroup // prevent the delete while they exist
}

// DeleteGroup is one related type's share of a preview.
type DeleteGroup struct {
	// Resource is the slug of a registered ModelAdmin, or "" for a type
	// the admin does not manage.
	Resource string
	// Label is the heading, a host string translated at render. ""
	// falls back to the resource's VerboseName.
	Label string
	// Objects is a sample; at most DeletePreviewSample are listed.
	Objects []any
	// Total is the full count, raised to len(Objects) when smaller.
	Total int
}

// DeletePreviewSample caps how many records of a group are listed.
const DeletePreviewSample = 10

// PreviewsDeletes reports whether modelAdmin implements DeletePreviewer.
func PreviewsDeletes(modelAdmin ModelAdmin) bool {
	_, ok := modelAdmin.(DeletePreviewer)
	return ok
}

// ResolvedDeleteGroup is a DeleteGroup made safe to show one principal.
type ResolvedDeleteGroup struct {
	// ModelAdmin is the group's registered resource; nil for a type the
	// admin does not manage.
	ModelAdmin ModelAdmin
	// Label is untranslated: the host's Label, else the VerboseName.
	Label string
	// Visible are the sampled objects the principal may view.
	Visible []any
	// Texts are the sampled objects of an unmanaged type, as plain text.
	Texts []string
	// Hidden counts sampled objects the principal may not view.
	Hidden int
	// More counts objects beyond the sample.
	More  int
	Total int
}

// ResolvedDeletePreview is what the pages render and the routes enforce.
type ResolvedDeletePreview struct {
	Cascades  []ResolvedDeleteGroup
	Protected []ResolvedDeleteGroup
	// DeniedTypes are the untranslated labels of cascaded types the
	// principal may not delete.
	DeniedTypes []string
	// Blocked is true when anything is protected or denied.
	Blocked bool
}

// ResolveDeletePreview asks modelAdmin what deleting objects takes with
// it and filters the answer for principal. Without the capability it
// returns an empty, non-blocking result and calls nothing.
func ResolveDeletePreview(ctx context.Context, admin *Admin, modelAdmin ModelAdmin, principal *Principal, objects []any) (ResolvedDeletePreview, error) {
	previewer, ok := modelAdmin.(DeletePreviewer)
	if !ok {
		return ResolvedDeletePreview{}, nil
	}
	preview, err := previewer.DeletePreview(ctx, objects)
	if err != nil {
		return ResolvedDeletePreview{}, err
	}
	var out ResolvedDeletePreview
	for _, group := range preview.Cascades {
		resolved, keep, err := resolveDeleteGroup(admin, principal, group)
		if err != nil {
			return ResolvedDeletePreview{}, err
		}
		if !keep {
			continue
		}
		out.Cascades = append(out.Cascades, resolved)
		target := resolved.ModelAdmin
		if target != nil && !previewAllows(admin, principal, target.CanDelete(), ResourcePermission(target.Slug(), "delete"), target) {
			out.DeniedTypes = append(out.DeniedTypes, resolved.Label)
		}
	}
	for _, group := range preview.Protected {
		resolved, keep, err := resolveDeleteGroup(admin, principal, group)
		if err != nil {
			return ResolvedDeletePreview{}, err
		}
		if keep {
			out.Protected = append(out.Protected, resolved)
		}
	}
	out.Blocked = len(out.Protected) > 0 || len(out.DeniedTypes) > 0
	return out, nil
}

// resolveDeleteGroup reports keep=false for an empty group.
func resolveDeleteGroup(admin *Admin, principal *Principal, group DeleteGroup) (ResolvedDeleteGroup, bool, error) {
	total := max(group.Total, len(group.Objects))
	if total == 0 {
		return ResolvedDeleteGroup{}, false, nil
	}
	sample := group.Objects
	if len(sample) > DeletePreviewSample {
		sample = sample[:DeletePreviewSample]
	}
	resolved := ResolvedDeleteGroup{Label: group.Label, Total: total, More: total - len(sample)}
	if group.Resource == "" {
		if group.Label == "" {
			return ResolvedDeleteGroup{}, false, fmt.Errorf("polyadmin: a DeleteGroup needs a Resource or a Label")
		}
		for _, obj := range sample {
			resolved.Texts = append(resolved.Texts, fmt.Sprint(obj))
		}
		return resolved, true, nil
	}
	target, ok := admin.GetModelAdmin(group.Resource)
	if !ok {
		return ResolvedDeleteGroup{}, false, fmt.Errorf("polyadmin: DeletePreview names unregistered resource %q", group.Resource)
	}
	resolved.ModelAdmin = target
	if resolved.Label == "" {
		resolved.Label = target.VerboseName()
	}
	// The detail page's two checks: the coarse one against the resource,
	// then the per-object one. A record failing either is counted, never
	// named -- the user learns something exists, not what.
	permission := ResourcePermission(target.Slug(), "view")
	viewable := previewAllows(admin, principal, target.CanView(), permission, target)
	for _, obj := range sample {
		if viewable && previewAllows(admin, principal, true, permission, obj) {
			resolved.Visible = append(resolved.Visible, obj)
		} else {
			resolved.Hidden++
		}
	}
	return resolved, true, nil
}

// previewAllows is the adapter's computePermissions rule for one check:
// the ModelAdmin's static toggle, then the Authorizer if there is one.
func previewAllows(admin *Admin, principal *Principal, capability bool, permission string, resource any) bool {
	if !capability {
		return false
	}
	return admin.Authorizer == nil || admin.Authorizer.Can(principal, permission, resource)
}

// SelectionFingerprint identifies a set of records: the hex SHA-256 of
// their primary keys' string forms, sorted and newline-joined. The
// delete_selected confirmation carries it so "all N matching" deletes
// exactly the set that was reviewed.
func SelectionFingerprint(modelAdmin ModelAdmin, objects []any) string {
	pks := make([]string, len(objects))
	for i, obj := range objects {
		pks[i] = fmt.Sprint(modelAdmin.GetPK(obj))
	}
	sort.Strings(pks)
	sum := sha256.Sum256([]byte(strings.Join(pks, "\n")))
	return hex.EncodeToString(sum[:])
}
