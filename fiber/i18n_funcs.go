package fiber

import (
	"encoding/json"
	"html/template"
	"maps"

	"github.com/MagicRodri/go-polyadmin/core"
)

// baseFuncs are the locale-independent template functions: `icon` for
// inline SVGs, `ui` for shadcn-derived class strings (see ui.go),
// `siteInitials` for avatar fallbacks, `dict`/`list` for multi-argument
// partial calls.
var baseFuncs = template.FuncMap{
	"icon":         iconHTML,
	"ui":           uiClasses,
	"siteInitials": siteInitials,
	"dict":         dictValues,
	"list":         listValues,
}

// templateFuncs is baseFuncs plus English translation functions, for any
// set built outside a Renderer (tests, the error page's last-resort
// fallback). Every template may call t/tn/tjs, so every set needs them.
var templateFuncs = localeFuncs(core.EnglishTranslator{}, core.DefaultLocale, nil)

// localeSwitcher is what the header and login page need to render the
// language switcher; nil when it is disabled or there is one locale.
type localeSwitcher struct {
	Options []core.LocaleOption
	Current string
}

// localeFuncs is baseFuncs plus translation functions bound to one
// locale. Bound at parse time, which is why each locale gets its own
// template sets (see Renderers).
//
//	{{t "msgid" args...}}             translated, then fmt-formatted
//	{{tn "one" "many" n args...}}      n picks the plural form
//	{{tjs "msgid" args...}}           a JSON string literal, for Alpine
//	                                  expressions inside HTML attributes
//	{{locale}}                         the locale code
//	{{localeSwitcher}}                 *localeSwitcher or nil
func localeFuncs(translator core.Translator, locale string, switcher *localeSwitcher) template.FuncMap {
	funcs := maps.Clone(baseFuncs)
	funcs["t"] = func(msgid string, args ...any) string {
		return translator.Translate(locale, msgid, args...)
	}
	funcs["tn"] = func(singular, plural string, n any, args ...any) string {
		return translator.TranslatePlural(locale, singular, plural, toInt(n), args...)
	}
	funcs["tjs"] = func(msgid string, args ...any) string {
		encoded, _ := json.Marshal(translator.Translate(locale, msgid, args...))
		return string(encoded)
	}
	funcs["locale"] = func() string { return locale }
	funcs["localeSwitcher"] = func() *localeSwitcher { return switcher }
	return funcs
}

// toInt accepts whatever integer type a template hands tn.
func toInt(n any) int {
	switch v := n.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
