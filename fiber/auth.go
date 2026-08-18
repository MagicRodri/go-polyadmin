package fiber

import "github.com/MagicRodri/go-polyadmin/core"

// authorize returns (principal, true) if the request may proceed, or
// (nil, false) if it was rejected -- the caller is responsible for
// writing the actual 401/403 response, since that's adapter-specific.
// If Admin wasn't given an authenticator/authorizer, this is a no-op:
// every request is treated as authenticated and permitted, matching
// the framework's behavior before auth was wired up.
func authorize(admin *core.Admin, request any, permission string, resource any) (*core.Principal, authResult) {
	var principal *core.Principal
	if admin.Authenticator != nil {
		principal = admin.Authenticator.Authenticate(request)
		if principal == nil {
			return nil, authUnauthenticated
		}
	}
	if admin.Authorizer != nil && !admin.Authorizer.Can(principal, permission, resource) {
		return nil, authForbidden
	}
	return principal, authOK
}

type authResult int

const (
	authOK authResult = iota
	authUnauthenticated
	authForbidden
)

// computePermissions combines a ModelAdmin's static capability toggles
// with the Authorizer's per-request decision -- used to
// decide which controls the templates show. The routes enforce this
// independently, so hiding a control here is a UX nicety, not the
// security boundary.
func computePermissions(admin *core.Admin, principal *core.Principal, modelAdmin core.ModelAdmin) permissions {
	slug := modelAdmin.Slug()
	allowed := func(capability bool, action string) bool {
		if !capability {
			return false
		}
		if admin.Authorizer == nil {
			return true
		}
		return admin.Authorizer.Can(principal, core.ResourcePermission(slug, action), modelAdmin)
	}
	return permissions{
		CanView:   allowed(modelAdmin.CanView(), "view"),
		CanCreate: allowed(modelAdmin.CanCreate(), "create"),
		CanUpdate: allowed(modelAdmin.CanUpdate(), "update"),
		CanDelete: allowed(modelAdmin.CanDelete(), "delete"),
		CanExport: allowed(modelAdmin.CanExport(), "export"),
	}
}
