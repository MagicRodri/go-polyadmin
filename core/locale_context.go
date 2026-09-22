package core

import "context"

// LocaleContext is the resolved locale of one request, with the
// translator that serves it. The adapters store it on the request so any
// code holding the request's context.Context can translate.
type LocaleContext struct {
	Locale     string
	Translator Translator
}

type localeContextKey struct{}

// LocaleContextKey is the key a *LocaleContext is stored under. The Fiber
// adapter sets it with c.Locals, which fasthttp's RequestCtx then returns
// from Value -- so c.Context() carries it into ModelAdmin methods.
var LocaleContextKey = localeContextKey{}

func WithLocaleContext(ctx context.Context, lc *LocaleContext) context.Context {
	return context.WithValue(ctx, LocaleContextKey, lc)
}

// LocaleContextOf returns the request's LocaleContext, or nil outside one.
func LocaleContextOf(ctx context.Context) *LocaleContext {
	if ctx == nil {
		return nil
	}
	lc, _ := ctx.Value(LocaleContextKey).(*LocaleContext)
	return lc
}

// Locale is the request's resolved locale, DefaultLocale outside a request.
func Locale(ctx context.Context) string {
	if lc := LocaleContextOf(ctx); lc != nil {
		return lc.Locale
	}
	return DefaultLocale
}

// T translates msgid into the request's locale -- for host code with an
// interpolated message, e.g. an action's result.
func T(ctx context.Context, msgid string, args ...any) string {
	if lc := LocaleContextOf(ctx); lc != nil && lc.Translator != nil {
		return lc.Translator.Translate(lc.Locale, msgid, args...)
	}
	return EnglishTranslator{}.Translate(DefaultLocale, msgid, args...)
}

// TN is T's plural form: n picks the form, args fill the placeholders.
func TN(ctx context.Context, singular, plural string, n int, args ...any) string {
	if lc := LocaleContextOf(ctx); lc != nil && lc.Translator != nil {
		return lc.Translator.TranslatePlural(lc.Locale, singular, plural, n, args...)
	}
	return EnglishTranslator{}.TranslatePlural(DefaultLocale, singular, plural, n, args...)
}

// N_ marks a string for catalog extraction without translating it, for
// strings declared as data and translated where they are rendered.
func N_(msgid string) string { return msgid }
