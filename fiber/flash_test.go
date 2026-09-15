package fiber

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A cookie value must be ASCII: a translated flash message is not, so it
// travels encoded and is decoded where it is read.
func TestRussianFlashRoundTripsInAnASCIICookie(t *testing.T) {
	app, _ := makeApp(t)
	resp := doLocalePostForm(t, app, "/admin/users/create", "ru", url.Values{"Email": {"a@example.com"}, "IsActive": {"true"}})
	if resp.StatusCode != 303 {
		t.Fatalf("create: got %d", resp.StatusCode)
	}
	value := ""
	for _, header := range resp.Header.Values("Set-Cookie") {
		for i := 0; i < len(header); i++ {
			if header[i] > 0x7e {
				t.Fatalf("Set-Cookie is not ASCII: %q", header)
			}
		}
		if rest, ok := strings.CutPrefix(header, flashCookieName+"="); ok {
			value, _, _ = strings.Cut(rest, ";")
		}
	}
	if value == "" {
		t.Fatal("no flash cookie was set")
	}
	page := body(t, doGet(t, app, "/admin/users", map[string]string{"Cookie": flashCookieName + "=" + value + "; " + localeCookieName + "=ru"}))
	if !strings.Contains(page, "запись создана") {
		t.Errorf("the Russian flash did not survive the round trip:\n%s", page)
	}
}

// flashText decodes the flash cookie a response sets and returns its
// messages' text, one per line ("" when it sets none).
func flashText(t *testing.T, resp *http.Response) string {
	t.Helper()
	var texts []string
	for _, header := range resp.Header.Values("Set-Cookie") {
		rest, ok := strings.CutPrefix(header, flashCookieName+"=")
		if !ok {
			continue
		}
		value, _, _ := strings.Cut(rest, ";")
		if value == "" {
			continue
		}
		raw, err := flashEncoding.DecodeString(value)
		if err != nil {
			t.Fatalf("flash cookie %q: %v", value, err)
		}
		var messages []flashMessage
		if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatalf("flash cookie %q: %v", raw, err)
		}
		for _, m := range messages {
			texts = append(texts, m.Text)
		}
	}
	return strings.Join(texts, "\n")
}
