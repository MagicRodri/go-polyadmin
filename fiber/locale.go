package fiber

import (
	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

const (
	localeCookieName = "admin_locale"
	localeFormField  = "locale"
	// One year, in seconds.
	localeCookieMaxAge = 365 * 24 * 60 * 60
)

type principalCacheKey struct{}
type rendererKey struct{}

// principalCache is one request's authentication result. A struct, not a
// bare *core.Principal, so "authenticated as nobody" is distinguishable
// from "not asked yet".
type principalCache struct{ principal *core.Principal }

// localeMiddleware resolves the request's locale before any handler runs
// -- CSRF failures included, so even that page is localised -- and stores
// it where core.Locale(c.Context()) finds it.
func localeMiddleware(admin *core.Admin, i18n *core.I18n, renderers *Renderers) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var resolver func() string
		if admin.LocaleResolver != nil {
			resolver = func() string { return admin.LocaleResolver(c, authenticate(admin, c)) }
		}
		locale := i18n.Resolve(c.Cookies(localeCookieName), resolver, c.Get(fiber.HeaderAcceptLanguage))
		c.Locals(core.LocaleContextKey, &core.LocaleContext{Locale: locale, Translator: i18n.Translator})
		c.Locals(rendererKey{}, renderers.byLocaleOrDefault(locale))
		return c.Next()
	}
}

// authenticate asks the Authenticator at most once per request: the
// locale middleware may already have, to hand a LocaleResolver the
// principal. Nil when no Authenticator is configured.
func authenticate(admin *core.Admin, c *fiber.Ctx) *core.Principal {
	if admin.Authenticator == nil {
		return nil
	}
	if cached, ok := c.Locals(principalCacheKey{}).(principalCache); ok {
		return cached.principal
	}
	principal := admin.Authenticator.Authenticate(c)
	c.Locals(principalCacheKey{}, principalCache{principal})
	return principal
}

// requestRenderer is the Renderer for this request's locale, or nil when
// the locale middleware did not run.
func requestRenderer(c *fiber.Ctx) *Renderer {
	r, _ := c.Locals(rendererKey{}).(*Renderer)
	return r
}

// tr translates a framework string built in a handler.
func tr(c *fiber.Ctx, msgid string, args ...any) string {
	return core.T(c.Context(), msgid, args...)
}

// trn is tr's plural form.
func trn(c *fiber.Ctx, singular, plural string, n int, args ...any) string {
	return core.TN(c.Context(), singular, plural, n, args...)
}
