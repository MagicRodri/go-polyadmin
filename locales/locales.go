// Package locales embeds the framework's translation catalogs: one JSON
// file per locale, keyed by the English msgid. A value is either the
// translation, or an object of CLDR plural forms (zero, one, two, few,
// many, other). English has no file -- the msgid is the English text.
package locales

import "embed"

//go:embed *.json
var FS embed.FS
