package core

import "testing"

func TestRegisterAndGetModelAdmin(t *testing.T) {
	userAdmin := newInMemoryUserAdmin()
	admin := New(WithModelAdmins(userAdmin))

	got, ok := admin.GetModelAdmin("users")
	if !ok {
		t.Fatalf("expected users to be registered")
	}
	if got.Slug() != "users" {
		t.Fatalf("got slug %q", got.Slug())
	}
}

func TestGetModelAdminMissing(t *testing.T) {
	admin := New()
	if _, ok := admin.GetModelAdmin("missing"); ok {
		t.Fatalf("expected missing slug to be absent")
	}
}

func TestDuplicateSlugPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected a panic on duplicate slug registration")
		}
	}()
	admin := New(WithModelAdmins(newInMemoryUserAdmin()))
	admin.Register(newInMemoryUserAdmin())
}

func TestModelAdminsPreservesRegistrationOrder(t *testing.T) {
	first := newInMemoryUserAdmin()
	first.SlugOverride = "a"
	second := newInMemoryUserAdmin()
	second.SlugOverride = "b"

	admin := New(WithModelAdmins(first, second))
	slugs := make([]string, 0, 2)
	for _, ma := range admin.ModelAdmins() {
		slugs = append(slugs, ma.Slug())
	}
	if slugs[0] != "a" || slugs[1] != "b" {
		t.Fatalf("got %v", slugs)
	}
}

func noopPageHandler(any) error { return nil }

func TestRouteRegistersAndReturnsPage(t *testing.T) {
	admin := New()
	page := admin.Route("/reports/contracts", noopPageHandler)
	if page.Path != "/reports/contracts" {
		t.Fatalf("got path %q", page.Path)
	}
	if len(admin.Pages()) != 1 || admin.Pages()[0].Path != page.Path {
		t.Fatalf("got pages %+v", admin.Pages())
	}
}

func TestRoutePanicsOnDuplicatePath(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected a panic on duplicate page path registration")
		}
	}()
	admin := New()
	admin.Route("/reports/contracts", noopPageHandler)
	admin.Route("/reports/contracts", noopPageHandler)
}

func TestPagesPreservesRegistrationOrder(t *testing.T) {
	admin := New()
	admin.Route("/a", noopPageHandler)
	admin.Route("/b", noopPageHandler)
	paths := make([]string, 0, 2)
	for _, p := range admin.Pages() {
		paths = append(paths, p.Path)
	}
	if paths[0] != "/a" || paths[1] != "/b" {
		t.Fatalf("got %v", paths)
	}
}

func TestOptionsSetDashboardAuthenticatorAuthorizer(t *testing.T) {
	dashboard := &Dashboard{Title: "Overview"}
	authenticator := DenyAllAuthenticator{}
	authorizer := DenyAllAuthorizer{}
	admin := New(
		WithDashboard(dashboard),
		WithAuthenticator(authenticator),
		WithAuthorizer(authorizer),
	)
	if admin.Dashboard != dashboard || admin.Authenticator != authenticator || admin.Authorizer != authorizer {
		t.Fatalf("got %+v", admin)
	}
}
