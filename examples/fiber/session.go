package main

// A reference LoginBackend: cookie sessions over an in-memory user
// table. Mirrors python-polyadmin/examples/fastapi/session.py.
//
// The admin owns the login *page* -- the form, the failure message, the
// redirect back to where you were headed. It does not own the session,
// which is why this file exists in the example rather than in the
// framework: where a session lives is an application decision, and the
// framework never needing a signing secret is what keeps it out of key
// management. Swap this for whatever your app already has (a session
// store, a JWT, an upstream IdP) and the admin's login page keeps
// working unchanged.
//
// One type implements both halves on purpose: core.LoginBackend writes
// the session, core.Authenticator reads it back. They have to agree on
// the format, so they belong together.

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

const (
	sessionCookieName = "admin_session"
	sessionTTL        = 12 * time.Hour

	// PBKDF2 parameters. 600k iterations of SHA-256 is OWASP's 2023
	// floor; it is deliberately slow, which is the point.
	pbkdf2Iterations = 600_000
	pbkdf2KeyLength  = 32
)

// demoAccount is one row of what would be a users table.
type demoAccount struct {
	email string
	// salt and hash, never the password. Derived at startup here only
	// because a runnable demo has to document its own credentials --
	// a real table stores these and has never seen the plaintext.
	salt []byte
	hash []byte

	displayName string
	isSuperuser bool
}

// The example's two accounts. Two, not one, so the difference between a
// superuser and an ordinary signed-in user is visible in the admin --
// sign in as viewer@example.com and SuperuserAuthorizer starts refusing
// things.
var demoCredentials = []struct {
	email, password, displayName string
	isSuperuser                  bool
}{
	{"admin@example.com", "polyadmin", "Demo Admin", true},
	{"viewer@example.com", "polyadmin", "Demo Viewer", false},
}

// ReadOnlyForNonSuperusers is the example's Authorizer.
//
// core.SuperuserAuthorizer would be the obvious choice and is the wrong
// one here: it is all-or-nothing, so a signed-in non-superuser is
// refused every permission -- including dashboard.view -- and sees a
// bare "Permission denied." on every page. That makes the second demo
// account useless, and makes the admin look broken rather than
// permissioned.
//
// This grants reads to anyone signed in and reserves writes for
// superusers, which is the smallest rule that actually shows the
// permission system working: sign in as viewer@example.com and the
// list's Add button, the row edit/delete controls and the custom Tools
// page all disappear, because computePermissions asks this same
// Authorizer which controls to render.
type ReadOnlyForNonSuperusers struct{}

func (ReadOnlyForNonSuperusers) Can(principal *core.Principal, permission string, resource any) bool {
	if principal == nil {
		return false
	}
	if principal.IsSuperuser {
		return true
	}
	if permission == core.DashboardView {
		return true
	}
	// "{slug}.{action}" -- see core.ResourcePermission. Reads only;
	// create/update/delete and the custom pages ("page.tools.broadcast",
	// which sends messages) fall through to false.
	switch {
	case strings.HasSuffix(permission, ".view"),
		strings.HasSuffix(permission, ".list"),
		strings.HasSuffix(permission, ".export"):
		return true
	}
	return false
}

// CookieSessionBackend signs a cookie holding the account's email and
// an expiry. Nothing else is stored: the cookie is the session, which
// is the smallest thing that can honestly be called one.
type CookieSessionBackend struct {
	secret   []byte
	accounts map[string]demoAccount
}

func NewCookieSessionBackend() *CookieSessionBackend {
	b := &CookieSessionBackend{secret: sessionSecret(), accounts: map[string]demoAccount{}}
	for _, c := range demoCredentials {
		salt := make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			log.Fatalf("generating a salt: %v", err)
		}
		hash, err := pbkdf2.Key(sha256.New, c.password, salt, pbkdf2Iterations, pbkdf2KeyLength)
		if err != nil {
			log.Fatalf("hashing the demo password: %v", err)
		}
		b.accounts[c.email] = demoAccount{
			email: c.email, salt: salt, hash: hash,
			displayName: c.displayName, isSuperuser: c.isSuperuser,
		}
	}
	return b
}

// sessionSecret keys the cookie signature. From the environment when
// set; otherwise a fresh random one, which means sessions do not
// survive a restart and would not be shared across replicas. That is
// the right default for a demo and the wrong one for anything else,
// hence the warning.
func sessionSecret() []byte {
	if fromEnv := os.Getenv("ADMIN_SESSION_SECRET"); fromEnv != "" {
		return []byte(fromEnv)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		log.Fatalf("generating a session secret: %v", err)
	}
	log.Println("example: ADMIN_SESSION_SECRET is unset -- using a random per-process secret, so sessions end when this process does")
	return secret
}

// VerifyCredentials answers "are these good?" and nothing else -- it
// does not touch the response. Establishing the session is
// BeginSession's job, which is what lets the admin refuse to sign
// someone in when the session store is broken.
func (b *CookieSessionBackend) VerifyCredentials(request any, identifier, password string) *core.Principal {
	account, found := b.accounts[strings.ToLower(strings.TrimSpace(identifier))]
	if !found {
		// Hash anyway. Returning early here would make "no such
		// account" measurably faster than "wrong password", which is
		// exactly the distinction core.LoginBackend asks
		// implementations not to leak.
		account = demoAccount{salt: make([]byte, 16), hash: make([]byte, pbkdf2KeyLength)}
	}
	candidate, err := pbkdf2.Key(sha256.New, password, account.salt, pbkdf2Iterations, pbkdf2KeyLength)
	if err != nil {
		return nil
	}
	if subtle.ConstantTimeCompare(candidate, account.hash) != 1 || !found {
		return nil
	}
	return &core.Principal{
		ID:          account.email,
		DisplayName: account.displayName,
		IsSuperuser: account.isSuperuser,
	}
}

func (b *CookieSessionBackend) BeginSession(request any, principal *core.Principal) error {
	c, ok := request.(*fiber.Ctx)
	if !ok {
		return fmt.Errorf("expected a *fiber.Ctx, got %T", request)
	}
	c.Cookie(&fiber.Cookie{
		Name:     sessionCookieName,
		Value:    b.sign(fmt.Sprint(principal.ID), time.Now().Add(sessionTTL)),
		Path:     "/",
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		Secure:   c.Protocol() == "https",
		Expires:  time.Now().Add(sessionTTL),
	})
	return nil
}

func (b *CookieSessionBackend) EndSession(request any) error {
	c, ok := request.(*fiber.Ctx)
	if !ok {
		return fmt.Errorf("expected a *fiber.Ctx, got %T", request)
	}
	c.Cookie(&fiber.Cookie{
		Name: sessionCookieName, Value: "", Path: "/",
		HTTPOnly: true, Expires: time.Now().Add(-time.Hour),
	})
	return nil
}

// Authenticate is the read side, and the reason this type implements
// both interfaces: it has to parse exactly what BeginSession wrote.
func (b *CookieSessionBackend) Authenticate(request any) *core.Principal {
	c, ok := request.(*fiber.Ctx)
	if !ok {
		return nil
	}
	email, valid := b.verify(c.Cookies(sessionCookieName))
	if !valid {
		return nil
	}
	account, found := b.accounts[email]
	if !found {
		// The signature was good but the account is gone -- deleted
		// since the cookie was issued. A valid signature over a stale
		// subject is still not an authenticated request.
		return nil
	}
	return &core.Principal{
		ID:          account.email,
		DisplayName: account.displayName,
		IsSuperuser: account.isSuperuser,
	}
}

// sign renders "<subject>|<expiry>|<mac>". The MAC covers the subject
// and the expiry together, so neither can be edited independently --
// signing only the subject would let anyone extend their own session
// indefinitely.
func (b *CookieSessionBackend) sign(subject string, expires time.Time) string {
	payload := subject + "|" + strconv.FormatInt(expires.Unix(), 10)
	return payload + "|" + hex.EncodeToString(b.mac(payload))
}

func (b *CookieSessionBackend) verify(cookie string) (string, bool) {
	if cookie == "" {
		return "", false
	}
	// Exactly three fields, or nothing: a subject containing "|" would
	// otherwise shift the expiry and MAC along and be checked against
	// the wrong values. This fails closed instead.
	parts := strings.Split(cookie, "|")
	if len(parts) != 3 {
		return "", false
	}
	subject, expiry, mac := parts[0], parts[1], parts[2]
	expected := b.mac(subject + "|" + expiry)
	given, err := hex.DecodeString(mac)
	if err != nil || !hmac.Equal(expected, given) {
		return "", false
	}
	// Expiry is checked only after the signature: an unsigned cookie's
	// expiry field is attacker-controlled and means nothing.
	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil || time.Now().After(time.Unix(seconds, 0)) {
		return "", false
	}
	return subject, true
}

func (b *CookieSessionBackend) mac(payload string) []byte {
	m := hmac.New(sha256.New, b.secret)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
