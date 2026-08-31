package core

import (
	"regexp"
	"testing"
)

var base64URL = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func TestNewCSRFTokenIsRandomAndURLSafe(t *testing.T) {
	a, b := NewCSRFToken(), NewCSRFToken()
	if a == b {
		t.Error("two tokens must not be equal")
	}
	// 32 bytes, base64url, unpadded.
	if len(a) != 43 {
		t.Errorf("got length %d, want 43: %q", len(a), a)
	}
	if !base64URL.MatchString(a) {
		t.Errorf("token is not base64url-safe: %q", a)
	}
}

func TestIsSafeMethod(t *testing.T) {
	for _, m := range []string{"GET", "HEAD", "OPTIONS", "TRACE", "get", "head"} {
		if !IsSafeMethod(m) {
			t.Errorf("%q should be safe", m)
		}
	}
	for _, m := range []string{"POST", "PUT", "PATCH", "DELETE", "post", ""} {
		if IsSafeMethod(m) {
			t.Errorf("%q should not be safe", m)
		}
	}
}

func TestCSRFTokensMatch(t *testing.T) {
	if !CSRFTokensMatch("abc", "abc") {
		t.Error("identical tokens must match")
	}
	// An empty cookie must never validate an empty submission -- that is
	// the "no token at all" case, which has to fail closed.
	for _, c := range [][2]string{{"abc", "abd"}, {"abc", "ab"}, {"", ""}, {"abc", ""}, {"", "abc"}} {
		if CSRFTokensMatch(c[0], c[1]) {
			t.Errorf("CSRFTokensMatch(%q, %q) should be false", c[0], c[1])
		}
	}
}

func TestSafeRedirectPath(t *testing.T) {
	const host, base, fallback = "admin.example.com", "/admin", "/admin/users"

	cases := []struct {
		name    string
		referer string
		want    string
	}{
		{"relative under base", "/admin/users?page=2", "/admin/users?page=2"},
		{"absolute same host", "https://admin.example.com/admin/users", "/admin/users"},
		{"base path itself", "/admin", "/admin"},
		{"empty referer", "", fallback},
		{"other host", "https://evil.example.com/admin/users", fallback},
		{"path outside base", "/etc/passwd", fallback},
		// "/adminX" must not pass a naive prefix check for "/admin".
		{"prefix lookalike", "/adminX/pwned", fallback},
		{"unparseable", "://not a url", fallback},
		{"protocol-relative to other host", "//evil.example.com/admin", fallback},
	}
	for _, tc := range cases {
		if got := SafeRedirectPath(tc.referer, host, base, fallback); got != tc.want {
			t.Errorf("%s: SafeRedirectPath(%q) = %q, want %q", tc.name, tc.referer, got, tc.want)
		}
	}
}
