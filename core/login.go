package core

// Login: the write side of authentication.
//
// Authenticator (auth.go) reads an existing session and answers "who is
// this request?". A LoginBackend is its counterpart: it answers "are
// these credentials good?" and then creates or destroys the session the
// Authenticator will go on to read.
//
// The split is deliberate, and it is what keeps this framework out of
// key management. The admin owns the login *page* -- the form, the
// error state, the redirect dance, the CSRF check -- because that is
// presentation, and presentation is what this framework is for. It does
// not own the session: it never mints a token, so it never needs a
// signing secret, and the WithSecretKey question the CSRF design
// deferred stays deferred. How a session is stored (a signed cookie, a
// server-side store, a JWT, an upstream IdP) remains the host
// application's decision, exactly as docs/authentication.md says
// identity itself does.
//
// See examples/fiber/session.go for a cookie-backed implementation.

// LoginBackend is what an application implements to turn on the admin's
// built-in login page. Registering one via WithLoginBackend is the
// switch: with no backend the login routes are not mounted at all and
// an unauthenticated request is answered with 401, exactly as before
// this existed.
//
// `request` is `any` for the same reason it is on Authenticator -- core
// must not know what a *fiber.Ctx is.
type LoginBackend interface {
	// VerifyCredentials returns the Principal these credentials
	// identify, or nil if they are not valid. Returning nil is an
	// ordinary outcome, not an error: the page re-renders with a
	// message.
	//
	// Implementations must compare passwords in constant time and must
	// not distinguish "no such user" from "wrong password" to the
	// caller -- the admin renders one message for both, and a backend
	// that leaks the difference through timing undoes that.
	VerifyCredentials(request any, identifier, password string) *Principal

	// BeginSession persists the sign-in so that the Authenticator
	// recognises subsequent requests. Called only after
	// VerifyCredentials has returned a non-nil Principal.
	BeginSession(request any, principal *Principal) error

	// EndSession clears it. Called by the logout route, and expected to
	// succeed even when there is no session to clear.
	EndSession(request any) error
}

// LoginPath and LogoutPath are the routes the adapters mount, relative
// to the admin's base path. They are constants rather than options: a
// configurable login path buys nothing (the page is the framework's,
// not the application's) and every link to it -- the redirect an
// unauthenticated request gets, the sidebar's sign-out button -- would
// have to thread the value through.
const (
	LoginPath  = "/login"
	LogoutPath = "/logout"
)

// NextQueryParam carries the URL an unauthenticated visitor was trying
// to reach, so signing in returns them there instead of dumping them on
// the dashboard.
const NextQueryParam = "next"

// SafeNextURL guards the open-redirect hole that a `next` parameter
// opens if it is echoed back into a Location header unchecked: an
// attacker who can get a victim to click
// /admin/login?next=https://evil.example gets the admin's own domain to
// bounce them somewhere hostile, after a real, successful login.
//
// The rule is that a destination must be a path inside this admin.
// Anything else -- a different origin, a scheme-relative //host URL, a
// path outside basePath, or an empty value -- falls back to basePath
// itself. Callers use the return value directly; there is no "invalid"
// signal to forget to check.
func SafeNextURL(next, basePath string) string {
	// Must be an absolute path, and must not be scheme-relative
	// ("//evil.example" is a URL, not a path, and browsers treat it as
	// one). Checking the first two bytes covers both.
	if len(next) < 1 || next[0] != '/' {
		return basePath
	}
	if len(next) > 1 && (next[1] == '/' || next[1] == '\\') {
		return basePath
	}
	// A backslash anywhere is rejected rather than normalised: some
	// browsers fold it to a forward slash, so "/\evil.example" can
	// escape even though it passes the checks above.
	for i := 0; i < len(next); i++ {
		if next[i] == '\\' {
			return basePath
		}
	}
	if !isUnderBasePath(next, basePath) {
		return basePath
	}
	return next
}

// isUnderBasePath reports whether path is basePath or sits beneath it.
// The boundary check matters: "/adminutes" starts with "/admin" as a
// string but is a different route entirely.
func isUnderBasePath(path, basePath string) bool {
	trimmed := basePath
	for len(trimmed) > 1 && trimmed[len(trimmed)-1] == '/' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if trimmed == "" || trimmed == "/" {
		return true
	}
	if len(path) < len(trimmed) || path[:len(trimmed)] != trimmed {
		return false
	}
	return len(path) == len(trimmed) || path[len(trimmed)] == '/' || path[len(trimmed)] == '?'
}
