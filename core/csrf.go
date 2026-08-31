package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
