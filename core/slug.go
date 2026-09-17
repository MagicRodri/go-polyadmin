package core

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Slugify turns a title into a URL-safe ASCII slug: "Café du Coin"
// becomes "cafe-du-coin" and "Привет мир" becomes "privet-mir". It backs
// PrepopulatedFields (docs/model-admin.md) and mirrors the client-side
// rule, so what the browser filled in and what a script here produces
// agree.
//
// Letters outside the transliteration table drop out, so a title written
// wholly in one -- CJK, say -- slugifies to "". SlugifyUnicode is the way
// out for an application that wants those letters kept.
func Slugify(value string) string {
	return slugify(transliterate(value))
}

// SlugifyUnicode is Slugify without the ASCII step: punctuation and
// spacing are still normalised, but the letters are kept as they are.
func SlugifyUnicode(value string) string {
	return slugify(value)
}

// slugify lowercases, collapses every run of non-alphanumerics into one
// hyphen, and trims the hyphens from both ends.
func slugify(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	pendingHyphen := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if pendingHyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingHyphen = false
			b.WriteRune(r)
		default:
			pendingHyphen = true
		}
	}
	return b.String()
}

// transliterate maps the letters PolyAdmin ships locales for onto ASCII:
// Latin-1's accents fall off under NFD, and Cyrillic goes through the
// table below. Anything left non-ASCII is dropped.
func transliterate(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	for _, r := range value {
		if replacement, ok := cyrillicASCII[r]; ok {
			b.WriteString(replacement)
			continue
		}
		if lower, ok := cyrillicASCII[unicode.ToLower(r)]; ok {
			b.WriteString(lower)
			continue
		}
		b.WriteRune(r)
	}
	// Decompose, then drop the combining marks an accent decomposed into.
	stripped, _, err := transform.String(
		transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC),
		b.String(),
	)
	if err != nil {
		stripped = b.String()
	}
	return strings.Map(func(r rune) rune {
		if r > unicode.MaxASCII {
			return -1
		}
		return r
	}, stripped)
}

// cyrillicASCII is the Russian alphabet in the BGN/PCGN-flavoured
// romanisation most readable to a Latin-alphabet reader. Keyed on the
// lowercase letter; transliterate lowercases before looking up, so one
// entry per letter serves both cases.
var cyrillicASCII = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}
