package core

import "testing"

func TestAllowAllAuthenticatesEveryRequest(t *testing.T) {
	authenticator := NewAllowAllAuthenticator(nil)
	if authenticator.Authenticate(nil) == nil {
		t.Fatalf("expected a principal")
	}
}

func TestAllowAllReturnsConfiguredPrincipal(t *testing.T) {
	principal := &Principal{ID: "u1", DisplayName: "Jane"}
	authenticator := NewAllowAllAuthenticator(principal)
	if authenticator.Authenticate(nil) != principal {
		t.Fatalf("expected the configured principal")
	}
}

func TestDenyAllNeverAuthenticates(t *testing.T) {
	var authenticator DenyAllAuthenticator
	if authenticator.Authenticate(nil) != nil {
		t.Fatalf("expected nil")
	}
}
