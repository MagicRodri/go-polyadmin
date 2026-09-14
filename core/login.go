package core

// Login: the write side of authentication. Authenticator answers "who is
// this request?"; a LoginBackend answers "are these credentials good?" and
// creates or destroys the session the Authenticator reads.
//
// The split keeps the framework out of key management. It owns the login
// page -- form, error state, redirect, CSRF -- because that is
// presentation. It never mints a token, so it never needs a signing
// secret, and how a session is stored stays the application's decision.
// See examples/fiber/session.go.

// LoginBackend turns on the admin's built-in login page. Registering one
// via WithLoginBackend is the switch: without it the login routes are
// never mounted and an unauthenticated request gets a 401. `request` is
// `any` because core must not know what a *fiber.Ctx is.
type LoginBackend interface {
	// VerifyCredentials returns the Principal these credentials identify, or
	// nil if they are not valid. nil is an ordinary outcome, not an error.
	//
	// Implementations must compare in constant time and must not distinguish
	// "no such user" from "wrong password": the admin renders one message for
	// both, and a backend leaking the difference through timing undoes that.
	VerifyCredentials(request any, identifier, password string) *Principal

	// BeginSession persists the sign-in so that the Authenticator
	// recognises subsequent requests. Called only after
	// VerifyCredentials has returned a non-nil Principal.
	BeginSession(request any, principal *Principal) error

	// EndSession clears it. Called by the logout route, and expected to
	// succeed even when there is no session to clear.
	EndSession(request any) error
}

// LoginPath and LogoutPath are mounted relative to the base path.
// Constants, not options: the page is the framework's, and every link to
// it would otherwise have to thread the value through.
const (
	LoginPath  = "/login"
	LogoutPath = "/logout"
)

// LocalePath is the language switcher's route, relative to the mount.
const LocalePath = "/locale"

// NextQueryParam carries the URL an unauthenticated visitor was trying
// to reach, so signing in returns them there instead of dumping them on
// the dashboard.
const NextQueryParam = "next"

// SafeNextURL guards the open redirect a `next` parameter opens if echoed
// into a Location header unchecked: ?next=https://evil.example would have
// the admin's own domain bounce the visitor somewhere hostile after a real
// login.
//
// A destination must be a path inside this admin; anything else falls back
// to basePath. Callers use the return value directly, so there is no
// "invalid" signal to forget to check.
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
