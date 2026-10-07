package fiber

import (
	"context"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

// The CSV goroutine outlives the handler, so it must not hold the
// request's context (a pooled *fasthttp.RequestCtx that Fiber reuses):
// it gets a fresh one carrying only the locale.
func TestExportContextCarriesOnlyTheLocale(t *testing.T) {
	type requestScoped struct{}
	lc := &core.LocaleContext{Locale: "fr", Translator: core.EnglishTranslator{}}
	request := context.WithValue(core.WithLocaleContext(context.Background(), lc), requestScoped{}, "pooled")

	ctx := exportContext(request)
	if core.LocaleContextOf(ctx) != lc {
		t.Error("the export context lost the request's locale")
	}
	if ctx.Value(requestScoped{}) != nil {
		t.Error("the export context still reaches the request's own context")
	}
}
