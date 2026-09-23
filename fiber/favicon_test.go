package fiber

import (
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

func TestModelAdminsOwnFaviconWinsOverTheSiteWide(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.NavFaviconURL = "https://example.com/user.ico"
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithSiteFaviconURL("https://example.com/site.ico"))
	app := newTestApp(t, admin)

	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, `<link rel="icon" href="https://example.com/user.ico">`) {
		t.Errorf("the model admin's own favicon did not win: %s", page)
	}
	if strings.Contains(page, "site.ico") {
		t.Error("the site-wide favicon leaked in alongside the model admin's own")
	}
}

func TestFaviconFallsBackToTheSiteWideWhenUnset(t *testing.T) {
	userAdmin := newTestUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin), core.WithSiteFaviconURL("https://example.com/site.ico"))
	app := newTestApp(t, admin)

	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, `<link rel="icon" href="https://example.com/site.ico">`) {
		t.Errorf("no fallback to the site-wide favicon: %s", page)
	}
}

func TestNoFaviconTagWhenNeitherIsSet(t *testing.T) {
	app, _ := makeApp(t)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if strings.Contains(page, `rel="icon"`) {
		t.Errorf("a favicon tag rendered despite nothing being configured: %s", page)
	}
}
