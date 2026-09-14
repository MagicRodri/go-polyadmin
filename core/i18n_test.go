package core

import (
	"context"
	"fmt"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

func catalogFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func mustTranslator(t *testing.T, catalogs ...map[string]string) *CatalogTranslator {
	t.Helper()
	var fss []fs.FS
	for _, c := range catalogs {
		fss = append(fss, catalogFS(c))
	}
	tr, err := NewCatalogTranslator(fss...)
	if err != nil {
		t.Fatalf("NewCatalogTranslator: %v", err)
	}
	return tr
}

func TestMissingTranslationFallsBackToEnglish(t *testing.T) {
	tr := mustTranslator(t, map[string]string{"fr.json": `{"Save": "Enregistrer"}`})
	if got := tr.Translate("fr", "Cancel"); got != "Cancel" {
		t.Errorf("got %q, want the English msgid", got)
	}
	if got := tr.Translate("de", "Save"); got != "Save" {
		t.Errorf("an unknown locale must fall back to English, got %q", got)
	}
}

func TestReservedWordMsgidsLoad(t *testing.T) {
	// go-i18n's own file loader rejects this file outright.
	tr := mustTranslator(t, map[string]string{"fr.json": `{"Description": "Descriptif", "Other": "Autre", "id": "identifiant", "Save": "Enregistrer"}`})
	for msgid, want := range map[string]string{"Description": "Descriptif", "Other": "Autre", "id": "identifiant", "Save": "Enregistrer"} {
		if got := tr.Translate("fr", msgid); got != want {
			t.Errorf("%q -> %q, want %q", msgid, got, want)
		}
	}
}

func TestLaterCatalogsOverrideEarlierOnes(t *testing.T) {
	tr := mustTranslator(t,
		map[string]string{"fr.json": `{"Save": "Enregistrer"}`},
		map[string]string{"fr.json": `{"Save": "Sauvegarder"}`},
	)
	if got := tr.Translate("fr", "Save"); got != "Sauvegarder" {
		t.Errorf("got %q, want the later catalog's entry", got)
	}
}

func TestRussianPluralForms(t *testing.T) {
	tr := mustTranslator(t, map[string]string{"ru.json": `{"%d record": {"one": "%d запись", "few": "%d записи", "many": "%d записей", "other": "%d записи"}}`})
	for n, want := range map[int]string{1: "1 запись", 2: "2 записи", 5: "5 записей", 11: "11 записей", 21: "21 запись", 22: "22 записи"} {
		if got := tr.TranslatePlural("ru", "%d record", "%d records", n, n); got != want {
			t.Errorf("n=%d: got %q, want %q", n, got, want)
		}
	}
}

func TestMissingPluralFallsBackToEnglish(t *testing.T) {
	tr := mustTranslator(t)
	if got := tr.TranslatePlural("ru", "%d record", "%d records", 5, 5); got != "5 records" {
		t.Errorf("got %q", got)
	}
	if got := tr.TranslatePlural("ru", "%d record", "%d records", 1, 1); got != "1 record" {
		t.Errorf("got %q", got)
	}
}

func TestLiteralPercentWithoutArgsIsUntouched(t *testing.T) {
	tr := mustTranslator(t)
	if got := tr.Translate("fr", "100% done"); got != "100% done" {
		t.Errorf("got %q", got)
	}
}

func TestMessagesAreNotTemplated(t *testing.T) {
	tr := mustTranslator(t, map[string]string{"fr.json": `{"a {{.X}} b": "c {{.X}} d"}`})
	if got := tr.Translate("fr", "a {{.X}} b"); got != "c {{.X}} d" {
		t.Errorf("got %q", got)
	}
	if got := tr.Translate("fr", "{{missing}}"); got != "{{missing}}" {
		t.Errorf("got %q", got)
	}
}

func TestArgsAreFormatted(t *testing.T) {
	tr := mustTranslator(t, map[string]string{"fr.json": `{"%s is required.": "%s est obligatoire."}`})
	if got := tr.Translate("fr", "%s is required.", "Email"); got != "Email est obligatoire." {
		t.Errorf("got %q", got)
	}
}

func TestLocalesListsEnglishFirstThenCatalogs(t *testing.T) {
	tr := mustTranslator(t, map[string]string{"ru.json": `{}`, "de.json": `{}`})
	got := tr.Locales()
	// Framework catalogs (fr, ru) are always embedded.
	want := []string{"en", "de", "fr", "ru"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPseudo(t *testing.T) {
	if got := Pseudo("Save"); got != "[Šàvé]" {
		t.Errorf("got %q", got)
	}
	if got := fmt.Sprintf(Pseudo("%d users"), 3); got != "[3 üšérš]" {
		t.Errorf("placeholders must survive: got %q", got)
	}
	if got := fmt.Sprintf(Pseudo("%[2]s and %[1]s"), "a", "b"); got != "[b àñd a]" {
		t.Errorf("indexed verbs must survive: got %q", got)
	}
}

func TestPseudoTranslatorOnlyWrapsThePseudoLocale(t *testing.T) {
	tr := pseudoTranslator{inner: mustTranslator(t, map[string]string{"fr.json": `{"Save": "Enregistrer"}`})}
	if got := tr.Translate(PseudoLocale, "Save"); got != "[Šàvé]" {
		t.Errorf("got %q", got)
	}
	if got := tr.Translate("fr", "Save"); got != "Enregistrer" {
		t.Errorf("other locales pass through: got %q", got)
	}
	if got := tr.TranslatePlural(PseudoLocale, "%d user", "%d users", 2, 2); got != "[2 üšérš]" {
		t.Errorf("got %q", got)
	}
	if got := tr.Translate(PseudoLocale, ""); got != "" {
		t.Errorf("empty stays empty: got %q", got)
	}
}

// TestPseudoTranslatorDoesNotDoubleWrapAnAlreadyPseudoString guards R5: a
// handler that translates a static message and then hands the result back
// through the translator a second time (e.g. fiber's tr(c, message) on an
// action's already-core.T-translated result) must get the same text back,
// not a second layer of brackets/accents.
func TestPseudoTranslatorDoesNotDoubleWrapAnAlreadyPseudoString(t *testing.T) {
	tr := pseudoTranslator{inner: mustTranslator(t)}
	already := Pseudo("Deleted 1 record.")
	if got := tr.Translate(PseudoLocale, already); got != already {
		t.Errorf("got %q, want %q unchanged", got, already)
	}
	if got := tr.TranslatePlural(PseudoLocale, already, already, 1); got != already {
		t.Errorf("got %q, want %q unchanged", got, already)
	}
}

func TestMatchLocale(t *testing.T) {
	supported := []string{"en", "fr", "ru", PseudoLocale}
	for candidate, want := range map[string]string{
		"fr": "fr", "FR": "fr", "fr-CA": "fr", "fr_CA": "fr", " ru ": "ru",
		"en-US": "en", "de": "", "": "", "not a tag!": "", "en-XA": PseudoLocale,
	} {
		if got := MatchLocale(supported, candidate); got != want {
			t.Errorf("MatchLocale(%q) = %q, want %q", candidate, got, want)
		}
	}
}

func TestMatchAcceptLanguage(t *testing.T) {
	supported := []string{"en", "fr", "ru", PseudoLocale}
	for header, want := range map[string]string{
		"fr-CA,fr;q=0.9,en;q=0.8": "fr",
		"de,ru;q=0.5":             "ru",
		"de":                      "",
		"en-US,en;q=0.9":          "en",
		"*":                       "",
		"ru;q=0.2,fr;q=0.8":       "fr",
		"fr;q=0":                  "",
		";;garbage":               "",
	} {
		if got := MatchAcceptLanguage(supported, header); got != want {
			t.Errorf("MatchAcceptLanguage(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestResolvePrecedence(t *testing.T) {
	i := &I18n{Default: "en", Supported: []string{"en", "fr", "ru"}}
	calls := 0
	resolver := func() string { calls++; return "ru" }

	if got := i.Resolve("fr", resolver, "en"); got != "fr" || calls != 0 {
		t.Errorf("cookie must win without consulting the resolver: got %q, %d calls", got, calls)
	}
	if got := i.Resolve("", resolver, "fr"); got != "ru" {
		t.Errorf("resolver must beat the header: got %q", got)
	}
	if got := i.Resolve("xx", func() string { return "" }, "fr"); got != "fr" {
		t.Errorf("an unsupported cookie and an empty resolver fall through to the header: got %q", got)
	}
	if got := i.Resolve("", nil, "de"); got != "en" {
		t.Errorf("nothing matches: got %q, want the default", got)
	}
}

func TestNewI18nDefaults(t *testing.T) {
	i, err := NewI18n(New())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(i.Supported, []string{"en", "fr", "ru"}) || i.Default != "en" {
		t.Errorf("got %v default %q", i.Supported, i.Default)
	}
}

func TestNewI18nRestrictsAndAddsPseudo(t *testing.T) {
	i, err := NewI18n(New(WithLocales("en", "fr"), WithPseudoLocale()))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(i.Supported, []string{"en", "fr", PseudoLocale}) {
		t.Errorf("got %v", i.Supported)
	}
	if got := i.Translator.Translate(PseudoLocale, "Save"); got != "[Šàvé]" {
		t.Errorf("pseudo translator not installed: %q", got)
	}
}

func TestNewI18nRejectsBadConfig(t *testing.T) {
	if _, err := NewI18n(New(WithLocales("en", "de"))); err == nil {
		t.Error("a locale with no catalog must be rejected")
	}
	if _, err := NewI18n(New(WithLocales("fr"), WithDefaultLocale("en"))); err == nil {
		t.Error("a default outside the supported set must be rejected")
	}
}

func TestLocaleNames(t *testing.T) {
	i, err := NewI18n(New(WithCatalogs(catalogFS(map[string]string{"de.json": `{}`})), WithLocaleNames(map[string]string{"de": "Deutsch"})))
	if err != nil {
		t.Fatal(err)
	}
	want := []LocaleOption{{"en", "English"}, {"de", "Deutsch"}, {"fr", "Français"}, {"ru", "Русский"}}
	if got := i.Options(); !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
	if got := (&I18n{}).Name("pt"); got != "pt" {
		t.Errorf("an unnamed locale shows its code: %q", got)
	}
}

func TestContextHelpers(t *testing.T) {
	if Locale(context.Background()) != DefaultLocale || T(context.Background(), "%d x", 2) != "2 x" {
		t.Error("without a LocaleContext the helpers are English")
	}
	if TN(context.Background(), "%d user", "%d users", 1, 1) != "1 user" {
		t.Error("English plural without a LocaleContext")
	}
	tr := mustTranslator(t, map[string]string{"fr.json": `{"Save": "Enregistrer"}`})
	ctx := WithLocaleContext(context.Background(), &LocaleContext{Locale: "fr", Translator: tr})
	if Locale(ctx) != "fr" || T(ctx, "Save") != "Enregistrer" {
		t.Errorf("got %q %q", Locale(ctx), T(ctx, "Save"))
	}
	if N_("Save") != "Save" {
		t.Error("N_ is the identity")
	}
}
