package core

// Principal is the authenticated identity behind a request.
type Principal struct {
	ID          any
	DisplayName string
	IsSuperuser bool
	Extra       map[string]any
}

// Authenticator turns a request into a Principal, or nil if
// unauthenticated. `request` is intentionally `any`: whatever the
// adapter (e.g. a Fiber *fiber.Ctx) passes through is opaque to core.
type Authenticator interface {
	Authenticate(request any) *Principal
}

// AllowAllAuthenticator authenticates every request as the same
// Principal. For local development and tests only -- never wire this
// into a real deployment.
type AllowAllAuthenticator struct {
	Principal *Principal
}

func NewAllowAllAuthenticator(principal *Principal) AllowAllAuthenticator {
	if principal == nil {
		// N_: the sidebar translates this default name (and only this
		// one -- a principal's own name is never translated).
		principal = &Principal{ID: "anonymous", DisplayName: N_("Anonymous"), IsSuperuser: true}
	}
	return AllowAllAuthenticator{Principal: principal}
}

func (a AllowAllAuthenticator) Authenticate(request any) *Principal { return a.Principal }

// DenyAllAuthenticator authenticates nobody. Useful for asserting a
// login gate works.
type DenyAllAuthenticator struct{}

func (DenyAllAuthenticator) Authenticate(request any) *Principal { return nil }
