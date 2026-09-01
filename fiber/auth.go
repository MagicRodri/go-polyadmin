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

// authorizeObject re-runs a permission check with the loaded record as
// the resource, so an Authorizer can answer "may this principal touch
// *this* record" and not only "may they touch this model at all".
//
// It is the second, narrower gate: the coarse check has already run
// (before the record was fetched, so an unauthorized principal never
// costs a lookup), and this one runs once there is an object to judge.
// With no Authorizer configured it permits, like every other check here.
func authorizeObject(admin *core.Admin, principal *core.Principal, permission string, obj any) bool {
	if admin.Authorizer == nil {
		return true
	}
	return admin.Authorizer.Can(principal, permission, obj)
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
// obj is the record in view, or nil on a list/create page. When present
// it is what the Authorizer is asked about, so per-object rules decide
// which controls a record's own pages show.
func computePermissions(admin *core.Admin, principal *core.Principal, modelAdmin core.ModelAdmin, obj any) permissions {
	slug := modelAdmin.Slug()
	allowed := func(capability bool, action string) bool {
		if !capability {
			return false
		}
		if admin.Authorizer == nil {
			return true
		}
		resource := any(modelAdmin)
		if obj != nil {
			resource = obj
		}
		return admin.Authorizer.Can(principal, core.ResourcePermission(slug, action), resource)
	}
	return permissions{
		CanView:   allowed(modelAdmin.CanView(), "view"),
		CanCreate: allowed(modelAdmin.CanCreate(), "create"),
		CanUpdate: allowed(modelAdmin.CanUpdate(), "update"),
		CanDelete: allowed(modelAdmin.CanDelete(), "delete"),
		CanExport: allowed(modelAdmin.CanExport(), "export"),
	}
}
