package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/fiber"

	"github.com/gofiber/fiber/v2"
)

// newOrganizationTestApp mounts a bare admin over a fresh in-memory
// repository -- no authenticator/authorizer configured, so every request
// is treated as authenticated and permitted (see fiber.authorize's own
// doc comment), which is exactly what the library's own handler tests
// rely on. This exercises the real HTTP pipeline: routing, CSRF, form
// parsing, Field.Validate, and OrganizationAdmin.Create/Update, not just
// the admin's own Go call.
//
// UserAdmin and RoleAdmin are registered alongside it purely so
// OrganizationAdmin's declared "users" inline (see NewOrganizationAdmin)
// resolves at mount time -- core.New validates every inline's target is
// itself a registered ModelAdmin.
func newOrganizationTestApp(t *testing.T) (*fiber.App, *OrganizationRepository) {
	t.Helper()
	repo := NewOrganizationRepository()
	users := NewUserRepository()
	roles := NewRoleRepository()
	admin := core.New(core.WithModelAdmins(
		NewOrganizationAdmin(repo),
		NewUserAdmin(users, repo, roles),
	))
	app := fiber.New()
	group := app.Group("/admin")
	if err := fiberadapter.Mount(group, admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app, repo
}

// postOrganizationForm double-submits a CSRF token as both cookie and
// header, matching what a real browser's meta-tag/htmx wiring does
// (see theme.html's htmx:configRequest listener and fiber/csrf.go).
func postOrganizationForm(t *testing.T, app *fiber.App, path string, form url.Values) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	token := core.NewCSRFToken()
	req.Header.Set("Cookie", core.CSRFCookieName+"="+token)
	req.Header.Set(core.CSRFHeaderName, token)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func TestCreateOrganizationPersistsFoundedAndBalance(t *testing.T) {
	app, repo := newOrganizationTestApp(t)

	form := url.Values{"Name": {"Acme"}, "Founded": {"2019-03-01"}, "Balance": {"500.5"}}
	resp := postOrganizationForm(t, app, "/admin/organizations/create", form)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("create: got %d, want %d (redirect)", resp.StatusCode, fiber.StatusSeeOther)
	}

	orgs := repo.List()
	if len(orgs) != 1 {
		t.Fatalf("got %d organizations, want 1", len(orgs))
	}
	org := orgs[0]
	if want := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC); !org.Founded.Equal(want) {
		t.Errorf("Founded = %v, want %v", org.Founded, want)
	}
	if org.Balance != 500.5 {
		t.Errorf("Balance = %v, want 500.5", org.Balance)
	}
}

func TestCreateOrganizationWithEmptyFoundedAndBalanceSavesZeroValues(t *testing.T) {
	app, repo := newOrganizationTestApp(t)

	form := url.Values{"Name": {"Acme"}, "Founded": {""}, "Balance": {""}}
	resp := postOrganizationForm(t, app, "/admin/organizations/create", form)
	if resp.StatusCode != fiber.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("create: got %d, want %d (redirect); body:\n%s", resp.StatusCode, fiber.StatusSeeOther, body)
	}

	orgs := repo.List()
	if len(orgs) != 1 {
		t.Fatalf("got %d organizations, want 1", len(orgs))
	}
	org := orgs[0]
	if !org.Founded.IsZero() {
		t.Errorf("Founded = %v, want the zero time", org.Founded)
	}
	if org.Balance != 0 {
		t.Errorf("Balance = %v, want 0", org.Balance)
	}
}

func TestCreateOrganizationRejectsGarbageAndPersistsNothing(t *testing.T) {
	app, repo := newOrganizationTestApp(t)

	form := url.Values{"Name": {"Acme"}, "Founded": {"not-a-date"}, "Balance": {"not-a-number"}}
	resp := postOrganizationForm(t, app, "/admin/organizations/create", form)
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("create: got %d, want %d", resp.StatusCode, fiber.StatusUnprocessableEntity)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Enter a valid date.") {
		t.Errorf("response is missing the Founded validation error:\n%s", body)
	}
	if !strings.Contains(string(body), "Enter a valid number.") {
		t.Errorf("response is missing the Balance validation error:\n%s", body)
	}
	if got := len(repo.List()); got != 0 {
		t.Errorf("a rejected create must not persist anything, got %d organizations", got)
	}
}

func TestUpdateOrganizationPersistsFoundedAndBalance(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	org := repo.Create("Acme", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), 100)

	form := url.Values{"Name": {"Acme"}, "Founded": {"2019-03-01"}, "Balance": {"500.5"}}
	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/edit"
	resp := postOrganizationForm(t, app, path, form)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("update: got %d, want %d (redirect)", resp.StatusCode, fiber.StatusSeeOther)
	}

	if want := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC); !org.Founded.Equal(want) {
		t.Errorf("Founded = %v, want %v", org.Founded, want)
	}
	if org.Balance != 500.5 {
		t.Errorf("Balance = %v, want 500.5", org.Balance)
	}
}

func TestUpdateOrganizationWithEmptyFoundedAndBalanceSavesZeroValues(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	org := repo.Create("Acme", time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC), 100)

	form := url.Values{"Name": {"Acme"}, "Founded": {""}, "Balance": {""}}
	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/edit"
	resp := postOrganizationForm(t, app, path, form)
	if resp.StatusCode != fiber.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("update: got %d, want %d (redirect); body:\n%s", resp.StatusCode, fiber.StatusSeeOther, body)
	}
	if !org.Founded.IsZero() {
		t.Errorf("Founded = %v, want the zero time", org.Founded)
	}
	if org.Balance != 0 {
		t.Errorf("Balance = %v, want 0", org.Balance)
	}
}

func TestUpdateOrganizationRejectsGarbageAndKeepsOriginalValues(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	original := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
	org := repo.Create("Acme", original, 100)

	form := url.Values{"Name": {"Acme"}, "Founded": {"nope"}, "Balance": {"nope"}}
	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/edit"
	resp := postOrganizationForm(t, app, path, form)
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("update: got %d, want %d", resp.StatusCode, fiber.StatusUnprocessableEntity)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "Enter a valid date.") {
		t.Errorf("response is missing the Founded validation error:\n%s", body)
	}
	if !strings.Contains(string(body), "Enter a valid number.") {
		t.Errorf("response is missing the Balance validation error:\n%s", body)
	}
	if !org.Founded.Equal(original) || org.Balance != 100 {
		t.Errorf("a rejected update must not change the record, got Founded=%v Balance=%v", org.Founded, org.Balance)
	}
}

// getOrganizationPage GETs an admin page and returns its body.
func getOrganizationPage(t *testing.T, app *fiber.App, path string) string {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("GET %s: got %d, want 200", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

var foundedInputValue = regexp.MustCompile(`name="Founded"\s+value="([^"]*)"`)

// foundedFormValue is what the edit form's Founded input would post back
// untouched: the value its <input type="date"> was filled with.
func foundedFormValue(t *testing.T, body string) string {
	t.Helper()
	m := foundedInputValue.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("the edit form has no Founded input:\n%s", body)
	}
	return m[1]
}

func TestEditFormFillsFoundedWithAnISODate(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	org := repo.Create("Acme", time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC), 100)

	body := getOrganizationPage(t, app, "/admin/organizations/"+strconv.Itoa(org.ID)+"/edit")
	if !strings.Contains(body, `value="2019-03-01"`) {
		t.Errorf("an <input type=date> only accepts YYYY-MM-DD; got Founded=%q", foundedFormValue(t, body))
	}
}

func TestEditingWithoutTouchingFoundedKeepsIt(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	founded := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
	org := repo.Create("Acme", founded, 100)
	path := "/admin/organizations/" + strconv.Itoa(org.ID) + "/edit"

	form := url.Values{"Name": {"Acme Renamed"}, "Founded": {foundedFormValue(t, getOrganizationPage(t, app, path))}, "Balance": {"100"}}
	resp := postOrganizationForm(t, app, path, form)
	if resp.StatusCode != fiber.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("update: got %d, want %d (redirect); body:\n%s", resp.StatusCode, fiber.StatusSeeOther, body)
	}
	if !org.Founded.Equal(founded) {
		t.Errorf("Founded = %v, want it kept at %v", org.Founded, founded)
	}
}

func TestZeroFoundedShowsAndRoundTripsAsEmpty(t *testing.T) {
	app, repo := newOrganizationTestApp(t)
	org := repo.Create("Acme", time.Time{}, 100)
	id := strconv.Itoa(org.ID)

	for _, path := range []string{"/admin/organizations", "/admin/organizations/" + id} {
		if body := getOrganizationPage(t, app, path); strings.Contains(body, "0001-01-01") {
			t.Errorf("%s shows the zero time as a date", path)
		}
	}
	edit := "/admin/organizations/" + id + "/edit"
	if got := foundedFormValue(t, getOrganizationPage(t, app, edit)); got != "" {
		t.Errorf("edit form Founded = %q, want empty", got)
	}
	form := url.Values{"Name": {"Acme"}, "Founded": {""}, "Balance": {"100"}}
	if resp := postOrganizationForm(t, app, edit, form); resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("update: got %d, want %d (redirect)", resp.StatusCode, fiber.StatusSeeOther)
	}
	if !org.Founded.IsZero() {
		t.Errorf("Founded = %v, want it still empty", org.Founded)
	}
}
