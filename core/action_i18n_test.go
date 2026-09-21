package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingDeleteAdmin struct{ BaseModelAdmin }

func (failingDeleteAdmin) Delete(ctx context.Context, obj any) error { return errors.New("disk full") }

func TestDeleteSelectedFailureMessageIsTranslated(t *testing.T) {
	translator, err := NewCatalogTranslator()
	if err != nil {
		t.Fatal(err)
	}
	for locale, want := range map[string]string{
		"en": "then failed",
		"fr": "puis échec",
		"ru": "затем сбой",
	} {
		ctx := WithLocaleContext(context.Background(), &LocaleContext{Locale: locale, Translator: translator})
		_, err := NewDeleteSelectedAction().Handler(ctx, failingDeleteAdmin{}, []any{struct{}{}}, nil)
		if err == nil {
			t.Fatalf("%s: expected an error", locale)
		}
		if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "disk full") {
			t.Errorf("%s: got %q, want it to contain %q and the cause", locale, err, want)
		}
	}
}
