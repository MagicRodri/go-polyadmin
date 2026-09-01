package fiber

import (
	"log"
	"net/url"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// The two messages the login page can show. invalidCredentialsMessage
// is deliberately one message for both "no such user" and "wrong
// password": telling them apart turns the form into an account
// enumerator. core.LoginBackend asks implementations not to
// distinguish them either, for the same reason.
const (
	invalidCredentialsMessage = "That email and password don't match an account."
	signedOutMessage          = "You have been signed out."
)

// loginURL builds the path an unauthenticated visitor is sent to,
// carrying where they were headed so signing in resumes it.
func loginURL(basePath, next string) string {
	target := basePath + core.LoginPath
	if next == "" {
		return target
	}
	return target + "?" + core.NextQueryParam + "=" + url.QueryEscape(next)
}

// requestedURL is the path (with query) the current request was for --
// what a redirect to login should come back to.
func requestedURL(c *fiber.Ctx) string {
	target := c.Path()
	if query := string(c.Request().URI().QueryString()); query != "" {
		target += "?" + query
	}
	return target
}

// handleLoginGet renders the form. It is one of only two routes in a
// mounted admin that run without authenticating -- the other is its
// POST -- since requiring a session to reach the page that creates one
// is a loop.
func handleLoginGet(admin *core.Admin, renderer *Renderer, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Already signed in: nothing here to do, so honour ?next= and
		// send them on rather than showing a form they'd have to
		// pointlessly fill in.
		if admin.Authenticator != nil && admin.Authenticator.Authenticate(c) != nil {
			return c.Redirect(core.SafeNextURL(c.Query(core.NextQueryParam), basePath), fiber.StatusSeeOther)
		}
		notice := ""
		if c.Query("signedout") == "1" {
			notice = signedOutMessage
		}
		return sendLoginPage(c, renderer, "", "", notice, fiber.StatusOK)
	}
}

// handleLoginPost verifies the submitted credentials and, on success,
// asks the backend to establish a session. CSRF is already enforced --
// csrfMiddleware covers every route under the mount, this one included,
// and the GET above is what mints the cookie the form echoes back.
func handleLoginPost(admin *core.Admin, renderer *Renderer, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		identifier := c.FormValue("identifier")
		password := c.FormValue("password")
		next := core.SafeNextURL(c.Query(core.NextQueryParam), basePath)

		principal := admin.LoginBackend.VerifyCredentials(c, identifier, password)
		if principal == nil {
			// 401, not 200: a failed sign-in is a failed sign-in, and
			// the status is what a log or a rate limiter in front of
			// this reads. The body is still the form.
			return sendLoginPage(c, renderer, identifier, invalidCredentialsMessage, "", fiber.StatusUnauthorized)
		}
		if err := admin.LoginBackend.BeginSession(c, principal); err != nil {
			// The credentials were right but the session could not be
			// stored, so the visitor is not signed in and must not be
			// told they are. Logged for the operator, generic on screen.
			log.Printf("polyadmin: BeginSession failed for %v: %v", principal.ID, err)
			return sendLoginPage(c, renderer, identifier, "Sign-in could not be completed. Please try again.", "", fiber.StatusInternalServerError)
		}
		return c.Redirect(next, fiber.StatusSeeOther)
	}
}

// handleLogout clears the session. POST-only (see the route table): a
// logout reachable by GET is a logout any <img src> on the internet can
// trigger.
func handleLogout(admin *core.Admin, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := admin.LoginBackend.EndSession(c); err != nil {
			// Nothing useful to offer the visitor here: they asked to
			// leave, and the most likely reason this failed is that
			// there was nothing to clear.
			log.Printf("polyadmin: EndSession failed: %v", err)
		}
		return redirectTo(c, basePath+core.LoginPath+"?signedout=1")
	}
}

func sendLoginPage(c *fiber.Ctx, renderer *Renderer, identifier, errorMessage, notice string, status int) error {
	html, err := renderer.RenderLogin(csrfToken(c), identifier, errorMessage, notice)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, fiber.MIMETextHTMLCharsetUTF8)
	return c.Status(status).SendString(html)
}
