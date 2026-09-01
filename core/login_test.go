package core

import "testing"

// SafeNextURL is the guard on an open redirect, so the cases that
// matter are the hostile ones: every rejection must land back on
// basePath rather than anywhere an attacker chose.
func TestSafeNextURLRejectsOffSiteDestinations(t *testing.T) {
	const basePath = "/admin"
	for _, next := range []string{
		"https://evil.example/phish",
		"http://evil.example",
		// Scheme-relative: a URL, not a path, however much it looks
		// like one.
		"//evil.example/phish",
		// Backslashes, which some browsers fold into forward slashes.
		"/\\evil.example",
		"\\\\evil.example",
		"/admin\\..\\..\\evil",
		// Real paths, but outside this admin.
		"/etc/passwd",
		"/other/app",
		// The prefix trap: starts with "/admin" as a string, different
		// route entirely.
		"/adminutes/secrets",
		// Nothing at all.
		"",
		"relative/path",
	} {
		if got := SafeNextURL(next, basePath); got != basePath {
			t.Errorf("SafeNextURL(%q) = %q, want the base path %q", next, got, basePath)
		}
	}
}

func TestSafeNextURLKeepsDestinationsInsideTheAdmin(t *testing.T) {
	const basePath = "/admin"
	for _, next := range []string{
		"/admin",
		"/admin/users",
		"/admin/users/7/edit",
		"/admin/users?page=3&sort=Email",
	} {
		if got := SafeNextURL(next, basePath); got != next {
			t.Errorf("SafeNextURL(%q) = %q, want it kept as-is", next, got)
		}
	}
}

// Mounting at the root makes every absolute path "inside" the admin,
// but the off-site checks still have to hold -- that is the case where
// a sloppy prefix check would let anything through.
func TestSafeNextURLAtRootStillRejectsOffSite(t *testing.T) {
	if got := SafeNextURL("/users", "/"); got != "/users" {
		t.Errorf("SafeNextURL(/users, /) = %q, want it kept", got)
	}
	if got := SafeNextURL("//evil.example", "/"); got != "/" {
		t.Errorf("SafeNextURL(//evil.example, /) = %q, want the base path", got)
	}
}

// A trailing slash on the mount point must not change which
// destinations are considered inside it.
func TestSafeNextURLIgnoresATrailingSlashOnBasePath(t *testing.T) {
	if got := SafeNextURL("/admin/users", "/admin/"); got != "/admin/users" {
		t.Errorf("SafeNextURL with basePath /admin/ = %q, want the path kept", got)
	}
}
