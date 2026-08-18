package core

import "fmt"

const DashboardView = "dashboard.view"

// ResourcePermission builds a standard "{slug}.{action}" permission
// name, e.g. ResourcePermission("users", "view") ==
// "users.view".
func ResourcePermission(slug, action string) string {
	return fmt.Sprintf("%s.%s", slug, action)
}

// Authorizer decides whether a Principal may exercise a permission
// against an optional resource. Authorization is
// always enforced server-side by the adapter; templates may use it to
// hide unavailable controls, but that's a UX nicety, never the
// security boundary.
type Authorizer interface {
	Can(principal *Principal, permission string, resource any) bool
}

// AllowAllAuthorizer grants every permission. For local development
// and tests only.
type AllowAllAuthorizer struct{}

func (AllowAllAuthorizer) Can(principal *Principal, permission string, resource any) bool {
	return true
}

// DenyAllAuthorizer denies every permission. Useful for asserting a
// gate works.
type DenyAllAuthorizer struct{}

func (DenyAllAuthorizer) Can(principal *Principal, permission string, resource any) bool {
	return false
}

// SuperuserAuthorizer grants every permission to superusers, denies
// everyone else -- a reasonable default before an application builds
// out granular roles.
type SuperuserAuthorizer struct{}

func (SuperuserAuthorizer) Can(principal *Principal, permission string, resource any) bool {
	return principal != nil && principal.IsSuperuser
}
