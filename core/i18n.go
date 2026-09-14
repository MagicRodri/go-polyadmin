package core

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/MagicRodri/go-polyadmin/locales"
)

const (
	// DefaultLocale is the locale every admin supports without a catalog:
	// msgids are the English text.
	DefaultLocale = "en"
	// PseudoLocale renders every translated string accented and
	// bracketed, so a page can be checked for strings that never went
	// through the translator. Enabled only by WithPseudoLocale.
	PseudoLocale = "en-XA"
)

// Translator translates framework and host strings alike, keyed by their
// English text. args are applied with fmt.Sprintf only when present.
type Translator interface {
	Translate(locale, msgid string, args ...any) string
	TranslatePlural(locale, singular, plural string, n int, args ...any) string
}

// LocaleResolver lets the host pick a request's locale -- from a stored
// user preference, say. principal is nil on the login and error pages.
// An empty or unsupported result falls through to Accept-Language.
type LocaleResolver func(request any, principal *Principal) string

// EnglishTranslator translates nothing: the English msgid, formatted.
type EnglishTranslator struct{}

func (EnglishTranslator) Translate(locale, msgid string, args ...any) string {
	return formatMessage(msgid, args)
}

func (EnglishTranslator) TranslatePlural(locale, singular, plural string, n int, args ...any) string {
	return formatMessage(englishPlural(singular, plural, n), args)
}

func formatMessage(text string, args []any) string {
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

func englishPlural(singular, plural string, n int) string {
	if n == 1 {
		return singular
	}
	return plural
}

// Delimiters that cannot occur in message text, so go-i18n's
// text/template pass never fires: msgids are English prose and may
// legitimately contain "{{".
const noTemplateLeft, noTemplateRight = "\x00[", "]\x00"

// CatalogTranslator is the default Translator: go-i18n, fed from the
// framework's embedded catalogs with any host catalogs layered on top.
type CatalogTranslator struct {
	bundle     *i18n.Bundle
	localizers map[string]*i18n.Localizer
	locales    []string
}

// NewCatalogTranslator loads the framework catalogs, then each host
// catalog filesystem in order. A later entry replaces an earlier one with
// the same msgid, so a host can reword the framework's strings too.
func NewCatalogTranslator(hostCatalogs ...fs.FS) (*CatalogTranslator, error) {
	t := &CatalogTranslator{bundle: i18n.NewBundle(language.English), localizers: map[string]*i18n.Localizer{}}
	found := map[string]bool{DefaultLocale: true}
	for _, fsys := range append([]fs.FS{locales.FS}, hostCatalogs...) {
		loaded, err := t.load(fsys)
		if err != nil {
			return nil, err
		}
		for _, locale := range loaded {
			found[locale] = true
		}
	}
	t.locales = sortLocales(slices.Collect(maps.Keys(found)))
	for _, locale := range t.locales {
		t.localizers[locale] = i18n.NewLocalizer(t.bundle, locale)
	}
	return t, nil
}

// load adds every *.json at the root of fsys. Parsed by hand rather than
// with go-i18n's file loader, which rejects a catalog whose msgids happen
// to be words it reserves ("Description", "Other", "id").
func (t *CatalogTranslator) load(fsys fs.FS) ([]string, error) {
	names, err := fs.Glob(fsys, "*.json")
	if err != nil {
		return nil, err
	}
	var loaded []string
	for _, name := range names {
		locale := strings.TrimSuffix(path.Base(name), ".json")
		tag, err := language.Parse(locale)
		if err != nil {
			return nil, fmt.Errorf("polyadmin: catalog %s: %w", name, err)
		}
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		messages, err := parseCatalog(raw)
		if err != nil {
			return nil, fmt.Errorf("polyadmin: catalog %s: %w", name, err)
		}
		if err := t.bundle.AddMessages(tag, messages...); err != nil {
			return nil, fmt.Errorf("polyadmin: catalog %s: %w", name, err)
		}
		loaded = append(loaded, locale)
	}
	return loaded, nil
}

func parseCatalog(raw []byte) ([]*i18n.Message, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	messages := make([]*i18n.Message, 0, len(entries))
	for id, value := range entries {
		m := &i18n.Message{ID: id, LeftDelim: noTemplateLeft, RightDelim: noTemplateRight}
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			m.Other = text
			messages = append(messages, m)
			continue
		}
		var forms map[string]string
		if err := json.Unmarshal(value, &forms); err != nil {
			return nil, fmt.Errorf("%q: want a string or an object of plural forms", id)
		}
		for form, text := range forms {
			switch form {
			case "zero":
				m.Zero = text
			case "one":
				m.One = text
			case "two":
				m.Two = text
			case "few":
				m.Few = text
			case "many":
				m.Many = text
			case "other":
				m.Other = text
			default:
				return nil, fmt.Errorf("%q: unknown plural form %q", id, form)
			}
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// Locales is DefaultLocale followed by every catalog's locale, sorted.
func (t *CatalogTranslator) Locales() []string { return slices.Clone(t.locales) }

func (t *CatalogTranslator) Translate(locale, msgid string, args ...any) string {
	text := t.localize(locale, &i18n.Message{ID: msgid, Other: msgid}, nil)
	if text == "" {
		text = msgid
	}
	return formatMessage(text, args)
}

func (t *CatalogTranslator) TranslatePlural(locale, singular, plural string, n int, args ...any) string {
	text := t.localize(locale, &i18n.Message{ID: singular, One: singular, Other: plural}, n)
	if text == "" {
		text = englishPlural(singular, plural, n)
	}
	return formatMessage(text, args)
}

// localize returns "" only when go-i18n could not produce anything. A
// missing message comes back as the fallback's text *with* a not-found
// error, which is exactly the English fallback wanted, so the error is
// deliberately not inspected.
func (t *CatalogTranslator) localize(locale string, fallback *i18n.Message, count any) string {
	fallback.LeftDelim, fallback.RightDelim = noTemplateLeft, noTemplateRight
	localizer, ok := t.localizers[locale]
	if !ok {
		localizer = t.localizers[DefaultLocale]
	}
	text, _ := localizer.Localize(&i18n.LocalizeConfig{DefaultMessage: fallback, PluralCount: count})
	return text
}

// sortLocales puts DefaultLocale first and the rest in alphabetical order.
func sortLocales(locales []string) []string {
	slices.SortFunc(locales, func(a, b string) int {
		switch {
		case a == DefaultLocale:
			return -1
		case b == DefaultLocale:
			return 1
		default:
			return strings.Compare(a, b)
		}
	})
	return locales
}

// pseudoTranslator wraps another Translator, rewriting PseudoLocale only.
type pseudoTranslator struct{ inner Translator }

func (p pseudoTranslator) Translate(locale, msgid string, args ...any) string {
	if locale != PseudoLocale {
		return p.inner.Translate(locale, msgid, args...)
	}
	if msgid == "" {
		return ""
	}
	if isAlreadyPseudo(msgid) {
		return formatMessage(msgid, args)
	}
	return formatMessage(Pseudo(p.inner.Translate(DefaultLocale, msgid)), args)
}

func (p pseudoTranslator) TranslatePlural(locale, singular, plural string, n int, args ...any) string {
	if locale != PseudoLocale {
		return p.inner.TranslatePlural(locale, singular, plural, n, args...)
	}
	if isAlreadyPseudo(singular) {
		return formatMessage(singular, args)
	}
	return formatMessage(Pseudo(p.inner.TranslatePlural(DefaultLocale, singular, plural, n)), args)
}

// isAlreadyPseudo reports whether s is already in Pseudo's output form.
// Reached when a msgid handed to Translate/TranslatePlural is itself the
// result of an earlier translation this same request already
// pseudo-wrapped -- a host's static action-result message re-translated
// by the framework (see fiber's tr(c, message)), or a validator error
// built with core.T and re-translated by Field.Validate. Composing two
// translation passes is meant to be a no-op when the first one already
// produced the final text (true of CatalogTranslator, which misses an
// unknown msgid and returns it unchanged); without this check,
// pseudoTranslator would instead wrap it a second time, and the sweep's
// nested-bracket stripping can't tell a legitimately double-translated
// string from a translated argument nested in a translated sentence.
func isAlreadyPseudo(s string) bool {
	return len(s) >= 2 && strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")
}

var (
	pseudoVerb    = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[a-zA-Z%]`)
	pseudoAccents = strings.NewReplacer(
		"a", "à", "e", "é", "i", "î", "o", "ö", "u", "ü", "y", "ý", "c", "ç", "n", "ñ", "s", "š", "z", "ž",
		"A", "À", "E", "É", "I", "Î", "O", "Ö", "U", "Ü", "Y", "Ý", "C", "Ç", "N", "Ñ", "S", "Š", "Z", "Ž",
	)
)

// Pseudo accents a string's letters and brackets it -- "Save" becomes
// "[Šàvé]" -- leaving fmt verbs intact so the result still formats.
func Pseudo(s string) string {
	var b strings.Builder
	b.WriteByte('[')
	last := 0
	for _, loc := range pseudoVerb.FindAllStringIndex(s, -1) {
		b.WriteString(pseudoAccents.Replace(s[last:loc[0]]))
		b.WriteString(s[loc[0]:loc[1]])
		last = loc[1]
	}
	b.WriteString(pseudoAccents.Replace(s[last:]))
	b.WriteByte(']')
	return b.String()
}

// MatchLocale returns the supported locale a candidate names, or "".
// Exact matches win (case-insensitively, "_" or "-"); otherwise a regional
// variant maps onto its base language (fr-CA -> fr) but one language is
// never mapped onto another. The pseudo-locale matches only exactly.
func MatchLocale(supported []string, candidate string) string {
	candidate = strings.ReplaceAll(strings.TrimSpace(candidate), "_", "-")
	if candidate == "" {
		return ""
	}
	for _, s := range supported {
		if strings.EqualFold(s, candidate) {
			return s
		}
	}
	tag, err := language.Parse(candidate)
	if err != nil {
		return ""
	}
	return matchTag(supported, tag)
}

// MatchAcceptLanguage returns the best supported locale for an
// Accept-Language header, or "". Tags are tried one at a time in q order,
// so a supported second choice beats a distant guess at the first.
func MatchAcceptLanguage(supported []string, header string) string {
	tags, weights, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return ""
	}
	for i, tag := range tags {
		if weights[i] <= 0 || tag == language.Und {
			continue
		}
		if s := matchTag(supported, tag); s != "" {
			return s
		}
	}
	return ""
}

func matchTag(supported []string, desired language.Tag) string {
	var names []string
	var tags []language.Tag
	for _, s := range supported {
		if s == PseudoLocale {
			continue
		}
		tag, err := language.Parse(s)
		if err != nil {
			continue
		}
		names, tags = append(names, s), append(tags, tag)
	}
	if len(tags) == 0 {
		return ""
	}
	_, index, confidence := language.NewMatcher(tags).Match(desired)
	if confidence < language.High {
		return ""
	}
	return names[index]
}

// LocaleOption is one entry in the language switcher.
type LocaleOption struct {
	Code string
	Name string
}

var builtinLocaleNames = map[string]string{
	"en":         "English",
	"fr":         "Français",
	"ru":         "Русский",
	PseudoLocale: "Pseudo (en-XA)",
}

// I18n is an Admin's internationalisation setup, built once at mount.
type I18n struct {
	Translator Translator
	Default    string
	// Supported is in switcher order: DefaultLocale's position as the
	// catalogs list it, then the rest, with PseudoLocale last if enabled.
	Supported []string
	names     map[string]string
}

// NewI18n resolves an Admin's i18n options into the translator and the
// supported locale set.
func NewI18n(a *Admin) (*I18n, error) {
	def := a.DefaultLocale
	if def == "" {
		def = DefaultLocale
	}
	translator := a.Translator
	available := []string{DefaultLocale}
	if translator == nil {
		catalogs, err := NewCatalogTranslator(a.Catalogs...)
		if err != nil {
			return nil, err
		}
		translator, available = catalogs, catalogs.Locales()
	}
	supported := available
	if len(a.Locales) > 0 {
		supported = nil
		for _, locale := range a.Locales {
			// A replacement Translator's locales are whatever the host
			// says; the catalog-backed one can only serve what it loaded.
			if a.Translator == nil && !slices.Contains(available, locale) {
				return nil, fmt.Errorf("polyadmin: locale %q has no catalog", locale)
			}
			supported = append(supported, locale)
		}
	}
	if !slices.Contains(supported, def) {
		return nil, fmt.Errorf("polyadmin: default locale %q is not among the supported locales %v", def, supported)
	}
	if a.PseudoLocale {
		supported = append(supported, PseudoLocale)
		translator = pseudoTranslator{inner: translator}
	}
	names := maps.Clone(builtinLocaleNames)
	maps.Copy(names, a.LocaleNames)
	return &I18n{Translator: translator, Default: def, Supported: supported, names: names}, nil
}

// Match returns the supported locale candidate names, or "".
func (i *I18n) Match(candidate string) string { return MatchLocale(i.Supported, candidate) }

// Resolve applies the resolution order: the switcher cookie, then the
// host's resolver, then Accept-Language, then the default. resolver is a
// thunk so that a request whose cookie already decides it never pays for
// authentication.
func (i *I18n) Resolve(cookie string, resolver func() string, acceptLanguage string) string {
	if locale := i.Match(cookie); locale != "" {
		return locale
	}
	if resolver != nil {
		if locale := i.Match(resolver()); locale != "" {
			return locale
		}
	}
	if locale := MatchAcceptLanguage(i.Supported, acceptLanguage); locale != "" {
		return locale
	}
	return i.Default
}

// Name is a locale's own name for itself, or its code when none is known.
func (i *I18n) Name(code string) string {
	if name := i.names[code]; name != "" {
		return name
	}
	if name := builtinLocaleNames[code]; name != "" {
		return name
	}
	return code
}

// Options lists the supported locales for the switcher.
func (i *I18n) Options() []LocaleOption {
	out := make([]LocaleOption, len(i.Supported))
	for n, code := range i.Supported {
		out[n] = LocaleOption{Code: code, Name: i.Name(code)}
	}
	return out
}
