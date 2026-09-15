package fiber

import (
	"strings"

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
//
// The admin_locale cookie counts only while the switcher is on (its route
// mounted): with it off nothing in the admin can change the cookie, so a
// leftover one must not override the resolver or the browser.
//
// Requests under staticPrefix (the WithStaticDir files; "" for none) are
// passed straight through: a stylesheet has no locale, and resolving one
// would run the LocaleResolver -- and so authenticate -- on every asset.
func localeMiddleware(admin *core.Admin, i18n *core.I18n, renderers *Renderers, staticPrefix string) fiber.Handler {
	switcherOn := switcherFor(admin, i18n, i18n.Default) != nil
	return func(c *fiber.Ctx) error {
		if staticPrefix != "" && strings.HasPrefix(c.Path(), staticPrefix) {
			return c.Next()
		}
		var resolver func() string
		if admin.LocaleResolver != nil {
			resolver = func() string { return admin.LocaleResolver(c, authenticate(admin, c)) }
		}
		cookie := ""
		if switcherOn {
			cookie = c.Cookies(localeCookieName)
		}
		locale := i18n.Resolve(cookie, resolver, c.Get(fiber.HeaderAcceptLanguage))
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

// handleLocalePost stores the switcher's choice and sends the user back
// where they were. An unsupported value is ignored rather than refused:
// the redirect is the same either way, and there is nothing to explain.
func handleLocalePost(i18n *core.I18n, basePath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if locale := i18n.Match(formValue(c, localeFormField)); locale != "" {
			c.Cookie(&fiber.Cookie{
				Name:     localeCookieName,
				Value:    locale,
				Path:     cookiePath(basePath),
				MaxAge:   localeCookieMaxAge,
				HTTPOnly: true,
				Secure:   c.Protocol() == "https",
				SameSite: fiber.CookieSameSiteLaxMode,
			})
		}
		// The Referer is attacker-controlled -- see core.SafeRedirectPath.
		// The fallback is the admin root: "/" when mounted at the site root.
		root := cookiePath(basePath)
		return redirectTo(c, core.SafeRedirectPath(c.Get("Referer"), string(c.Request().Host()), basePath, root))
	}
}

// cookiePath is the switcher cookie's Path: the admin mount, or the site
// root when mounted at the root itself.
func cookiePath(basePath string) string {
	if basePath == "" {
		return "/"
	}
	return basePath
}

// tr translates a framework string built in a handler.
func tr(c *fiber.Ctx, msgid string, args ...any) string {
	return core.T(c.Context(), msgid, args...)
}

// trn is tr's plural form.
func trn(c *fiber.Ctx, singular, plural string, n int, args ...any) string {
	return core.TN(c.Context(), singular, plural, n, args...)
}
