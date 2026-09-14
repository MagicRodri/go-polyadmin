package fiber

import (
	"reflect"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestBuildNavFlatLinkUsesModelAdminIcon(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavIcon = "table"
	admin := core.New(core.WithModelAdmins(userAdmin))
	renderer, err := NewRenderer(admin, "/admin")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	nav := renderer.buildNav("")
	if len(nav) != 1 || nav[0].IsGroup || nav[0].Link.Icon != "table" {
		t.Fatalf("got %+v", nav)
	}
}

func TestBuildNavFlatLinkDefaultsToCollectionIcon(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newTestUserAdmin()))
	renderer, err := NewRenderer(admin, "/admin")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	nav := renderer.buildNav("")
	if nav[0].Link.Icon != "collection" {
		t.Fatalf("got %q", nav[0].Link.Icon)
	}
}

func TestBuildNavNestedLinkKeepsItsOwnIcon(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavCategory = "Directory"
	userAdmin.NavIcon = "table"
	admin := core.New(core.WithModelAdmins(userAdmin))
	renderer, err := NewRenderer(admin, "/admin")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	nav := renderer.buildNav("")
	if !nav[0].IsGroup || nav[0].Group.Links[0].Icon != "table" {
		t.Fatalf("got %+v", nav[0])
	}
}

func TestBuildNavGroupUsesFixedFolderIcon(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavCategory = "Directory"
	admin := core.New(core.WithModelAdmins(userAdmin))
	renderer, err := NewRenderer(admin, "/admin")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	nav := renderer.buildNav("")
	if nav[0].Group.Icon != "folder" {
		t.Fatalf("got %q", nav[0].Group.Icon)
	}
}

// breadcrumbRenderer is an English Renderer mounted at /admin, for the
// breadcrumb builders, which translate and so hang off a Renderer.
func breadcrumbRenderer(t *testing.T, modelAdmins ...core.ModelAdmin) *Renderer {
	t.Helper()
	renderer, err := NewRenderer(core.New(core.WithModelAdmins(modelAdmins...)), "/admin")
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	return renderer
}

func TestCategoryBreadcrumbEmptyForNoCategory(t *testing.T) {
	if got := breadcrumbRenderer(t).categoryBreadcrumb(""); got != nil {
		t.Fatalf("got %v", got)
	}
}

func TestCategoryBreadcrumbIsPlainNonActiveNonLinkCrumb(t *testing.T) {
	got := breadcrumbRenderer(t).categoryBreadcrumb("Directory")
	want := []breadcrumb{{Label: "Directory"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got[0].Active {
		t.Fatalf("category crumb must not be Active")
	}
}

func TestListBreadcrumbsIncludeCategoryAndMarkLastActive(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavCategory = "Directory"
	crumbs := breadcrumbRenderer(t, userAdmin).listBreadcrumbs(userAdmin)
	want := []breadcrumb{
		{Label: "Directory"},
		{Label: "User", Active: true},
	}
	if !reflect.DeepEqual(crumbs, want) {
		t.Fatalf("got %+v, want %+v", crumbs, want)
	}
}

func TestListBreadcrumbsOmitCategoryWhenUnset(t *testing.T) {
	userAdmin := newTestUserAdmin()
	crumbs := breadcrumbRenderer(t, userAdmin).listBreadcrumbs(userAdmin)
	want := []breadcrumb{{Label: "User", Active: true}}
	if !reflect.DeepEqual(crumbs, want) {
		t.Fatalf("got %+v, want %+v", crumbs, want)
	}
}

func TestDetailBreadcrumbsIncludeCategoryBeforeResourceLink(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavCategory = "Directory"
	user := userAdmin.createUser("john@example.com", true)
	crumbs := breadcrumbRenderer(t, userAdmin).detailBreadcrumbs(userAdmin, user)
	if len(crumbs) != 3 {
		t.Fatalf("got %+v", crumbs)
	}
	if crumbs[0] != (breadcrumb{Label: "Directory"}) {
		t.Fatalf("got first crumb %+v", crumbs[0])
	}
	if !crumbs[2].Active {
		t.Fatalf("expected the last (object label) crumb to be Active, got %+v", crumbs[2])
	}
}

func TestDeleteBreadcrumbsLastCrumbIsActive(t *testing.T) {
	userAdmin := newTestUserAdmin()
	user := userAdmin.createUser("john@example.com", true)
	crumbs := breadcrumbRenderer(t, userAdmin).deleteBreadcrumbs(userAdmin, user)
	last := crumbs[len(crumbs)-1]
	if last.Label != "Delete" || !last.Active {
		t.Fatalf("got %+v", last)
	}
}

func TestFormBreadcrumbsNewAndEditLastCrumbIsActive(t *testing.T) {
	userAdmin := newTestUserAdmin()
	renderer := breadcrumbRenderer(t, userAdmin)
	newCrumbs := renderer.formBreadcrumbs(userAdmin, nil)
	if last := newCrumbs[len(newCrumbs)-1]; last.Label != "New" || !last.Active {
		t.Fatalf("got %+v", last)
	}

	user := userAdmin.createUser("john@example.com", true)
	editCrumbs := renderer.formBreadcrumbs(userAdmin, user)
	if last := editCrumbs[len(editCrumbs)-1]; last.Label != "Edit" || !last.Active {
		t.Fatalf("got %+v", last)
	}
}
