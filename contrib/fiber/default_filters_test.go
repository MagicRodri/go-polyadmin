package fiber

import (
	"context"
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/gofiber/fiber/v2"
)

type defaultedUserAdmin struct{ *testUserAdmin }

func (defaultedUserAdmin) DefaultFilters() map[string]any { return map[string]any{"IsActive": true} }

func (a defaultedUserAdmin) ListPage(ctx context.Context, req core.ListRequest) ([]any, int, error) {
	value, chosen := req.Filters["IsActive"]
	if !chosen {
		value = "true"
	}
	var out []any
	for _, u := range a.store {
		if value == "" || (value == "true") == u.IsActive {
			out = append(out, u)
		}
	}
	return out, len(out), nil
}

func defaultedApp(t *testing.T) *fiber.App {
	t.Helper()
	users := newTestUserAdmin()
	users.DeclaredFilters = []core.Filter{core.NewBooleanFilter("IsActive")}
	users.createUser("on@x.com", true)
	users.createUser("off@x.com", false)
	return newTestApp(t, core.New(core.WithModelAdmins(defaultedUserAdmin{users})))
}

// pageLinks is every href on a page, HTML-unescaped (what a browser
// requests) and also query-unescaped (what it means).
func pageLinks(page string) (raw, decoded []string) {
	for _, m := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(page, -1) {
		link := html.UnescapeString(m[1])
		meaning, err := url.QueryUnescape(link)
		if err != nil {
			meaning = link
		}
		raw, decoded = append(raw, link), append(decoded, meaning)
	}
	return raw, decoded
}

var spelledOutAll = regexp.MustCompile(`filter\[IsActive\]=(&|$)`)

func TestTheAllLinkOfADefaultedFilterIsSpelledOut(t *testing.T) {
	app := defaultedApp(t)
	page := readBody(t, doGet(t, app, "/admin/users", nil).Body)
	if !strings.Contains(page, "on@x.com") || strings.Contains(page, "off@x.com") {
		t.Fatal("the default did not apply")
	}
	raw, decoded := pageLinks(page)
	all := ""
	for i := range decoded {
		if strings.HasPrefix(decoded[i], "/admin/users?") && spelledOutAll.MatchString(decoded[i]) {
			all = raw[i]
		}
	}
	if all == "" {
		t.Fatal("no spelled-out All link")
	}
	everything := readBody(t, doGet(t, app, all, nil).Body)
	if !strings.Contains(everything, "on@x.com") || !strings.Contains(everything, "off@x.com") {
		t.Fatal("All did not show every row")
	}
}

func TestAChosenAllSurvivesSortLinks(t *testing.T) {
	page := readBody(t, doGet(t, defaultedApp(t), "/admin/users?filter%5BIsActive%5D=", nil).Body)
	_, decoded := pageLinks(page)
	sorts := 0
	for _, link := range decoded {
		if strings.Contains(link, "sort=") {
			sorts++
			if !spelledOutAll.MatchString(link) {
				t.Errorf("a sort link dropped the chosen All: %s", link)
			}
		}
	}
	if sorts == 0 {
		t.Fatal("no sort links on the page")
	}
}

func TestAChosenAllSurvivesExport(t *testing.T) {
	query := exportQuery(core.ListRequest{Filters: map[string]string{"IsActive": ""}})
	if !spelledOutAll.MatchString(query) {
		t.Fatalf("export query %q", query)
	}
}

func TestFilterChoiceSelection(t *testing.T) {
	cases := []struct {
		value, current    string
		chosen, defaulted bool
		want              bool
	}{
		{"", "", false, false, true},
		{"", "", false, true, false},
		{"", "", true, true, true},
		{"true", "true", true, true, true},
		{"false", "true", true, true, false},
	}
	for _, c := range cases {
		if got := filterChoiceSelected(c.value, c.current, c.chosen, c.defaulted); got != c.want {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}
