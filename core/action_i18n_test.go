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
		"en": "then failed: ",
		"fr": "puis échec : ",
		"ru": "затем сбой: ",
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

type refusingDeleteAdmin struct{ BaseModelAdmin }

func (refusingDeleteAdmin) Delete(ctx context.Context, obj any) error {
	if obj == "second" {
		return &RecordFormError{Errors: map[string][]string{"": {"Still referenced."}}}
	}
	return nil
}

func TestDeleteSelectedStopsWithAnActionErrorCarryingDone(t *testing.T) {
	_, err := NewDeleteSelectedAction().Handler(context.Background(), refusingDeleteAdmin{}, []any{"first", "second", "third"}, nil)
	var actionErr *ActionError
	if !errors.As(err, &actionErr) {
		t.Fatalf("got %T %v", err, err)
	}
	if actionErr.Message != "Deleted 1 of 3, then failed: Still referenced." || len(actionErr.Done) != 1 || actionErr.Done[0] != "first" {
		t.Fatalf("got %+v", actionErr)
	}
}
