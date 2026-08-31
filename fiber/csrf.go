package fiber

import (
	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/gofiber/fiber/v2"
)

// csrfLocalsKeyType keys the request's token in Locals. A private type
// rather than a string so it cannot collide with any other package's
// key.
type csrfLocalsKeyType struct{}

var csrfLocalsKey = csrfLocalsKeyType{}

// csrfToken returns the token minted or read for this request. Always
// non-empty inside a mounted admin: csrfMiddleware runs before every
// handler.
func csrfToken(c *fiber.Ctx) string {
	token, _ := c.Locals(csrfLocalsKey).(string)
	return token
}

// csrfMiddleware implements double-submit CSRF protection and sets the
// clickjacking headers. See
// .idea/superpowers/specs/2026-09-01-csrf-hardening-design.md.
//
// Every request gets a token cookie (minted if absent) so that any page
// can render the value; unsafe requests must additionally echo it back,
// in the X-CSRF-Token header or the _csrf form field. The header path is
// not a convenience: two routes are bodyless DELETEs that a hidden form
// field cannot reach at all.
func csrfMiddleware(admin *core.Admin, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Unconditional, and deliberately outside the DisableCSRF check:
		// framing is a different attack from forgery.
		c.Set("X-Frame-Options", "DENY")
		c.Set("Content-Security-Policy", "frame-ancestors 'none'")

		token := c.Cookies(core.CSRFCookieName)
		if token == "" {
			token = core.NewCSRFToken()
			c.Cookie(&fiber.Cookie{
				Name:     core.CSRFCookieName,
				Value:    token,
				Path:     basePath,
				HTTPOnly: true,
				SameSite: fiber.CookieSameSiteLaxMode,
				// Secure only over TLS: a Secure cookie is not sent over
				// plain HTTP, which would break running the example on a
				// LAN address. Behind a proxy this needs
				// X-Forwarded-Proto to be forwarded.
				Secure: c.Protocol() == "https",
			})
		}
		c.Locals(csrfLocalsKey, token)

		if admin.DisableCSRF || core.IsSafeMethod(c.Method()) {
			return c.Next()
		}

		submitted := c.Get(core.CSRFHeaderName)
		if submitted == "" {
			// Fiber reads this from the buffered body, so the handler can
			// still parse the form afterwards.
			submitted = c.FormValue(core.CSRFFieldName)
		}
		if !core.CSRFTokensMatch(submitted, token) {
			return c.Status(fiber.StatusForbidden).
				SendString("CSRF token missing or invalid. Reload the page and try again.")
		}
		return c.Next()
	}
}
