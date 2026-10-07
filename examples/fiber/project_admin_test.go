package main

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/contrib/fiber"
	"github.com/gofiber/fiber/v2"
)

func newProjectsTestApp(t *testing.T) *fiber.App {
	t.Helper()
	db, err := openProjectsDB()
	if err != nil {
		t.Fatal(err)
	}
	clients, projects, err := newProjectAdmins(db)
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	if err := fiberadapter.Mount(app.Group("/admin"), core.New(core.WithModelAdmins(clients, projects)), "/admin"); err != nil {
		t.Fatal(err)
	}
	return app
}

func projectPage(t *testing.T, app *fiber.App, path string) string {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("%s: status %d", path, resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func TestProjectsListHidesInactiveByDefault(t *testing.T) {
	page := projectPage(t, newProjectsTestApp(t), "/admin/projects")
	if !strings.Contains(page, "Apollo") || strings.Contains(page, "Cascade") {
		t.Fatal("the Active default did not apply")
	}
	if !strings.Contains(page, "2026-11-01") || !strings.Contains(page, "Northwind") {
		t.Fatal("the due date or the client does not render")
	}
}

func TestProjectWithoutClientRenders(t *testing.T) {
	app := newProjectsTestApp(t)
	page := projectPage(t, app, "/admin/projects?search=Fjord")
	if !strings.Contains(page, "Fjord") {
		t.Fatal("Fjord is not listed")
	}
	projectPage(t, app, "/admin/projects/6")
	projectPage(t, app, "/admin/projects/6/edit")
}

func TestClientsList(t *testing.T) {
	page := projectPage(t, newProjectsTestApp(t), "/admin/clients")
	for _, name := range []string{"Northwind", "Contoso", "Umbrella"} {
		if !strings.Contains(page, name) {
			t.Errorf("%s missing", name)
		}
	}
}
