package fiber

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

// TestImageFieldRendersAThumbnailLinkedToTheFullImage: list, detail, and
// inline rows all go through fieldValueHTML, so a thumbnail here covers
// all three at once -- see render_helpers.go's doc comment.
func TestImageFieldRendersAThumbnailLinkedToTheFullImage(t *testing.T) {
	const url = "https://example.com/pic.jpg"
	got := renderValue(t, core.FieldTypeImage, url)

	thumb, err := uiClasses("image", "thumb")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(got, `<img src="`+url+`"`) || !strings.Contains(got, thumb) {
		t.Errorf("no themed thumbnail in %q", got)
	}
	// Clicking through to the original is the only way to see it full
	// size -- there is no lightbox.
	if !strings.Contains(got, `<a href="`+url+`" target="_blank"`) {
		t.Errorf("the thumbnail does not link to the full image: %q", got)
	}
}

// TestImageFieldThumbnailFallsBackOnLoadError: a dead URL would otherwise
// show the browser's bare broken-image icon, which is what the rest of
// the admin avoids (see TestALongWidgetScrollsInsideItsOwnCard's themed
// scrollbar for the same reasoning applied to overflow).
func TestImageFieldThumbnailFallsBackOnLoadError(t *testing.T) {
	got := renderValue(t, core.FieldTypeImage, "https://example.com/dead.jpg")

	fallback, err := uiClasses("image", "fallback")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(got, fallback) {
		t.Errorf("no fallback element for a failed load: %q", got)
	}
	if !strings.Contains(got, `style="display:none"`) {
		t.Errorf("the fallback is not hidden until the image actually fails: %q", got)
	}
	if !strings.Contains(got, `onerror=`) {
		t.Errorf("nothing swaps the fallback in when the image fails: %q", got)
	}
}

// TestImageFieldFallbackStaysHiddenDespiteInlineFlex: the fallback's own
// class sets display:inline-flex for once it's shown, but the browser's
// default `[hidden] { display: none }` is a lowest-priority UA rule --
// any author stylesheet, including a Tailwind utility class on the same
// element, wins over it regardless of order, so a plain `hidden`
// attribute is not actually hidden here. Confirmed live with Playwright
// against the real Tailwind CDN build: the fallback rendered a real
// 32x32 box even though `hidden` was present. An inline style, not the
// attribute, has to be the one holding it closed.
func TestImageFieldFallbackStaysHiddenDespiteInlineFlex(t *testing.T) {
	got := renderValue(t, core.FieldTypeImage, "https://example.com/pic.jpg")
	if strings.Contains(got, ` hidden>`) || strings.Contains(got, ` hidden `) {
		t.Errorf("the fallback relies on the hidden attribute, which a display utility class on the same element overrides: %q", got)
	}
}

// TestImageFieldFormInputIsAURLInput: no upload handling yet -- the form
// just takes the image's URL, with the browser's own URL validation.
func TestImageFieldFormInputIsAURLInput(t *testing.T) {
	got := renderInput(t, core.FieldTypeImage, "")
	if !strings.Contains(got, `type="url"`) {
		t.Errorf("image field's form input is not a URL input: %q", got)
	}
}
