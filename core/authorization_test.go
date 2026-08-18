package core

import "testing"

func TestResourcePermissionNaming(t *testing.T) {
	if got := ResourcePermission("users", "view"); got != "users.view" {
		t.Fatalf("got %q", got)
	}
}

func TestAllowAllGrantsEverything(t *testing.T) {
	var authorizer AllowAllAuthorizer
	if !authorizer.Can(nil, "users.delete", nil) {
		t.Fatalf("expected true")
	}
}

func TestDenyAllDeniesEverything(t *testing.T) {
	var authorizer DenyAllAuthorizer
	principal := &Principal{ID: 1, IsSuperuser: true}
	if authorizer.Can(principal, "users.view", nil) {
		t.Fatalf("expected false")
	}
}

func TestSuperuserAuthorizer(t *testing.T) {
	var authorizer SuperuserAuthorizer
	if !authorizer.Can(&Principal{ID: 1, IsSuperuser: true}, "users.delete", nil) {
		t.Fatalf("expected superuser to be allowed")
	}
	if authorizer.Can(&Principal{ID: 2, IsSuperuser: false}, "users.delete", nil) {
		t.Fatalf("expected non-superuser to be denied")
	}
	if authorizer.Can(nil, "users.delete", nil) {
		t.Fatalf("expected nil principal to be denied")
	}
}
