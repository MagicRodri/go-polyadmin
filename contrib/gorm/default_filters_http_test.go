package gorm

import (
	"html"
	"io"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	polyfiber "github.com/MagicRodri/go-polyadmin/contrib/fiber"
	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/gofiber/fiber/v2"
)

func TestTheAllLinkOfADefaultedFilterShowsEveryRow(t *testing.T) {
	db := openDB(t)
	yes, no := true, false
	mustCreate(t, db, &Author{Name: "on-author", Active: &yes}, &Author{Name: "off-author", Active: &no})
	admin, err := New[Author](db, core.BaseModelAdmin{ModelName: "Author", DisplayFields: []string{"ID", "Name", "Active"},
		DeclaredFilters: []core.Filter{core.NewBooleanFilter("Active")}}, WithDefaultFilters(map[string]any{"Active": true}))
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	if err := polyfiber.Mount(app.Group("/admin"), core.New(core.WithModelAdmins(admin)), "/admin"); err != nil {
		t.Fatal(err)
	}
	get := func(path string) string {
		resp, err := app.Test(httptest.NewRequest("GET", path, nil), -1)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	page := get("/admin/authors")
	if !strings.Contains(page, "on-author") || strings.Contains(page, "off-author") {
		t.Fatal("the default did not apply")
	}
	var all string
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(page, -1) {
		link := html.UnescapeString(m[1])
		meaning, _ := url.QueryUnescape(link)
		if strings.HasPrefix(meaning, "/admin/authors?") && regexp.MustCompile(`filter\[Active\]=(&|$)`).MatchString(meaning) {
			all = link
		}
	}
	if all == "" {
		t.Fatal("no spelled-out All link")
	}
	everything := get(all)
	if !strings.Contains(everything, "on-author") || !strings.Contains(everything, "off-author") {
		t.Fatal("All did not show every row")
	}
}
