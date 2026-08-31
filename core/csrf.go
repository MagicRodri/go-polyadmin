package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/url"
	"strings"
)

// The wire names for the CSRF token, shared by both adapters and by the
// Python implementation -- see polyadmin/core/csrf.py. Changing one
// without the other silently breaks every form in the other language.
const (
	CSRFCookieName = "admin_csrf"
	CSRFHeaderName = "X-CSRF-Token"
	CSRFFieldName  = "_csrf"
)

// NewCSRFToken returns 32 crypto-random bytes as unpadded base64url (43
// characters). It panics rather than returning an error: a system whose
// CSPRNG is unavailable cannot serve an admin safely, and every caller
// would only turn the error into the same panic.
func NewCSRFToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic("polyadmin: crypto/rand unavailable: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// IsSafeMethod reports whether a method is read-only per RFC 9110 and so
// needs no CSRF token.
func IsSafeMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}

// CSRFTokensMatch compares in constant time. Empty on either side is
// always false: "no cookie" and "no submitted token" must fail closed
// rather than match each other.
func CSRFTokensMatch(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// SafeRedirectPath validates a client-supplied Referer before it is used
// as a redirect target, returning fallback when it cannot be trusted.
//
// A raw Referer is attacker-controlled: without this, an action could be
// made to bounce the signed-in admin to any site on the internet. The
// returned value is always a path (never absolute), so the redirect can
// only ever land inside this admin.
func SafeRedirectPath(referer, host, basePath, fallback string) string {
	if referer == "" {
		return fallback
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		return fallback
	}
	// A non-empty host must be ours. This also rejects protocol-relative
	// "//evil.example.com/admin", which parses with an empty scheme but a
	// foreign host.
	if parsed.Host != "" && parsed.Host != host {
		return fallback
	}
	// Exact match, or a child path -- "/adminX" must not pass for "/admin".
	if parsed.Path != basePath && !strings.HasPrefix(parsed.Path, basePath+"/") {
		return fallback
	}
	if parsed.RawQuery != "" {
		return parsed.Path + "?" + parsed.RawQuery
	}
	return parsed.Path
}
