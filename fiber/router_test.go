package fiber

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
	"github.com/xuri/excelize/v2"
)

type testUser struct {
	ID       int
	Email    string
	IsActive bool
}

type testUserAdmin struct {
	core.BaseModelAdmin
	store  map[int]*testUser
	nextID int
}

func newTestUserAdmin() *testUserAdmin {
	return &testUserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "IsActive"},
			FormFieldNames:   []string{"Email", "IsActive"},
			SearchFieldNames: []string{"Email"},
			DeclaredFields: []core.Field{
				core.NewField("IsActive", core.FieldTypeBoolean, core.WithDefault(true)),
				core.NewField("Email", core.FieldTypeEmail, core.WithRequired()),
			},
		},
		store:  make(map[int]*testUser),
		nextID: 1,
	}
}

func (a *testUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, u := range a.store {
		out = append(out, u)
	}
	return out, nil
}

func (a *testUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	u, ok := a.store[id]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (a *testUserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	u := &testUser{ID: a.nextID, Email: data["Email"].(string), IsActive: data["IsActive"].(bool)}
	a.store[u.ID] = u
	a.nextID++
	return u, nil
}

func (a *testUserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	u := obj.(*testUser)
	if email, ok := data["Email"].(string); ok {
		u.Email = email
	}
	if active, ok := data["IsActive"].(bool); ok {
		u.IsActive = active
	}
	return u, nil
}

func (a *testUserAdmin) Delete(ctx context.Context, obj any) error {
	delete(a.store, obj.(*testUser).ID)
	return nil
}

func (a *testUserAdmin) createUser(email string, active bool) *testUser {
	u := &testUser{ID: a.nextID, Email: email, IsActive: active}
	a.store[u.ID] = u
	a.nextID++
	return u
}

func newTestApp(t *testing.T, admin *core.Admin) *fiber.App {
	t.Helper()
	app := fiber.New()
	group := app.Group("/admin")
	if err := Mount(group, admin, "/admin"); err != nil {
		t.Fatalf("mount: %v", err)
	}
	return app
}

func makeApp(t *testing.T) (*fiber.App, *testUserAdmin) {
	t.Helper()
	userAdmin := newTestUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin))
	return newTestApp(t, admin), userAdmin
}

func doGet(t *testing.T, app *fiber.App, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("GET", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func doPostForm(t *testing.T, app *fiber.App, path string, form url.Values, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func doDelete(t *testing.T, app *fiber.App, path string, headers map[string]string) *http.Response {
	t.Helper()
	req := httptest.NewRequest("DELETE", path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return resp
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}

func TestIndexRedirectsToFirstResource(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/", nil)
	if resp.StatusCode != fiber.StatusTemporaryRedirect {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin/users" {
		t.Fatalf("got %q", loc)
	}
}

func TestListViewRendersHTML(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)

	resp := doGet(t, app, "/admin/users", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "john@example.com") {
		t.Fatalf("got %s", text)
	}
}

func TestCreateGetRendersEmptyForm(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users/create", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "Create User") {
		t.Fatalf("got %s", text)
	}
}

func TestCreatePostRedirectsToDetail(t *testing.T) {
	app, userAdmin := makeApp(t)
	form := url.Values{"Email": {"new@example.com"}, "IsActive": {"true"}}
	resp := doPostForm(t, app, "/admin/users/create", form, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d body=%s", resp.StatusCode, body(t, resp))
	}
	if len(userAdmin.store) != 1 {
		t.Fatalf("got %d users", len(userAdmin.store))
	}
}

func TestCreatePostWithoutCheckboxIsFalse(t *testing.T) {
	app, userAdmin := makeApp(t)
	form := url.Values{"Email": {"new@example.com"}}
	doPostForm(t, app, "/admin/users/create", form, nil)
	for _, u := range userAdmin.store {
		if u.IsActive {
			t.Fatalf("expected IsActive false")
		}
	}
}

func TestCreatePostInvalidRerendersFormWithErrors(t *testing.T) {
	app, userAdmin := makeApp(t)
	form := url.Values{"Email": {""}}
	resp := doPostForm(t, app, "/admin/users/create", form, nil)
	if resp.StatusCode != fiber.StatusUnprocessableEntity {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "is required") {
		t.Fatalf("got %s", text)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected no users created")
	}
}

func TestDetailView(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	resp := doGet(t, app, "/admin/users/"+strconv.Itoa(user.ID), nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "john@example.com") {
		t.Fatalf("got %s", text)
	}
}

func TestDetailViewMissingIs404(t *testing.T) {
	app, _ := makeApp(t)
	resp := doGet(t, app, "/admin/users/999", nil)
	if resp.StatusCode != 404 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

// Regression test for a fmt.Sprintf("%s", ...) verb mismatch against an
// int PK (produces the literal string "%!s(int=1)" instead of "1"),
// which corrupted the edit form's action/hx-post URLs and made Save
// silently no-op (htmx doesn't swap on a non-2xx/404 response). Posts
// straight to the URL the *rendered form* advertises, rather than one
// built independently by the test, so a broken action attribute would
// actually be caught.
func TestEditFormActionUsesCorrectPK(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	resp := doGet(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/edit", nil)
	text := body(t, resp)
	wantAction := `action="/admin/users/` + strconv.Itoa(user.ID) + `/edit"`
	wantHxPost := `hx-post="/admin/users/` + strconv.Itoa(user.ID) + `/edit"`
	if !strings.Contains(text, wantAction) {
		t.Fatalf("expected %s, got %s", wantAction, text)
	}
	if !strings.Contains(text, wantHxPost) {
		t.Fatalf("expected %s, got %s", wantHxPost, text)
	}
	if strings.Contains(text, "%!s(") {
		t.Fatalf("form action contains a fmt verb-mismatch artifact: %s", text)
	}
}

func TestEditPostUpdatesAndRedirects(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	form := url.Values{"Email": {"updated@example.com"}, "IsActive": {"true"}}
	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/edit", form, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if user.Email != "updated@example.com" {
		t.Fatalf("got %q", user.Email)
	}
}

func TestDeleteGetRendersConfirmation(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	resp := doGet(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/delete", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); !strings.Contains(text, "Are you sure") {
		t.Fatalf("got %s", text)
	}
}

func TestDeletePostRemovesAndRedirectsToList(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	resp := doPostForm(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/delete", url.Values{}, nil)
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin/users" {
		t.Fatalf("got %q", loc)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected user deleted")
	}
}

func TestHTMXDeleteRemovesRowWithoutRedirect(t *testing.T) {
	app, userAdmin := makeApp(t)
	user := userAdmin.createUser("john@example.com", true)

	resp := doDelete(t, app, "/admin/users/"+strconv.Itoa(user.ID)+"/delete", map[string]string{"HX-Request": "true"})
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); text != "" {
		t.Fatalf("got %q", text)
	}
	if len(userAdmin.store) != 0 {
		t.Fatalf("expected user deleted")
	}
}

func TestHTMXListRequestReturnsFragmentNotFullPage(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)

	resp := doGet(t, app, "/admin/users", map[string]string{"HX-Request": "true"})
	text := body(t, resp)
	if strings.Contains(text, "<html") {
		t.Fatalf("expected fragment, got full page: %s", text)
	}
	if !strings.Contains(text, `id="resource-list"`) {
		t.Fatalf("got %s", text)
	}
}

func TestHTMXCreatePostReturnsHXRedirectHeader(t *testing.T) {
	app, userAdmin := makeApp(t)
	form := url.Values{"Email": {"new@example.com"}}
	resp := doPostForm(t, app, "/admin/users/create", form, map[string]string{"HX-Request": "true"})
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	var pk int
	for id := range userAdmin.store {
		pk = id
	}
	if got := resp.Header.Get("HX-Redirect"); got != "/admin/users/"+strconv.Itoa(pk) {
		t.Fatalf("got %q", got)
	}
}

func TestSearchFiltersRows(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)
	userAdmin.createUser("mary@example.com", true)

	resp := doGet(t, app, "/admin/users?search=john", nil)
	text := body(t, resp)
	if !strings.Contains(text, "john@example.com") || strings.Contains(text, "mary@example.com") {
		t.Fatalf("got %s", text)
	}
}

func TestSortOrdersRows(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("bbb@example.com", true)
	userAdmin.createUser("aaa@example.com", true)

	resp := doGet(t, app, "/admin/users?sort=Email", nil)
	text := body(t, resp)
	if strings.Index(text, "aaa@example.com") > strings.Index(text, "bbb@example.com") {
		t.Fatalf("got %s", text)
	}
}

func TestDisabledOperationsAreNotRouted(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.BaseModelAdmin.DisableCreate = true
	userAdmin.BaseModelAdmin.DisableUpdate = true
	userAdmin.BaseModelAdmin.DisableDelete = true
	admin := core.New(core.WithModelAdmins(userAdmin))
	app := newTestApp(t, admin)

	if resp := doGet(t, app, "/admin/users/1/edit", nil); resp.StatusCode != 404 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if resp := doPostForm(t, app, "/admin/users/1/delete", url.Values{}, nil); resp.StatusCode != 404 {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

func TestCSVExportDownloadsAllRows(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)
	userAdmin.createUser("mary@example.com", true)

	resp := doGet(t, app, "/admin/users/export/csv", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("got %q", ct)
	}
	text := body(t, resp)
	if !strings.Contains(text, "john@example.com") || !strings.Contains(text, "mary@example.com") {
		t.Fatalf("got %s", text)
	}
}

func TestXLSXExportDownloadsAllRows(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("john@example.com", true)
	userAdmin.createUser("mary@example.com", true)

	resp := doGet(t, app, "/admin/users/export/xlsx", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	wantType := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if ct := resp.Header.Get("Content-Type"); ct != wantType {
		t.Fatalf("got %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, `filename="users.xlsx"`) {
		t.Fatalf("got %q", cd)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a valid xlsx file: %v", err)
	}
	sheet := file.GetSheetName(0)
	if sheet != "User" {
		t.Fatalf("expected sheet named %q, got %q", "User", sheet)
	}
	rows, err := file.GetRows(sheet)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	if len(rows) != 3 { // header + 2 users
		t.Fatalf("got %d rows: %v", len(rows), rows)
	}
	var emails []string
	for _, row := range rows[1:] {
		emails = append(emails, row[1]) // ID, Email, IsActive
	}
	if !slices.Contains(emails, "john@example.com") || !slices.Contains(emails, "mary@example.com") {
		t.Fatalf("got %v", emails)
	}
}

func TestRegisteredPageRouteIsMounted(t *testing.T) {
	userAdmin := newTestUserAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin))
	admin.Route("/tools/broadcast", PageHandler(func(pc *PageContext) error {
		return pc.C.SendString("broadcast page")
	}))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/tools/broadcast", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if text := body(t, resp); text != "broadcast page" {
		t.Fatalf("got %q", text)
	}
}
