package fiber

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

type testOrg struct {
	ID   int
	Name string
}

type testOrgAdmin struct {
	core.BaseModelAdmin
	store map[int]*testOrg
}

func newTestOrgAdmin() *testOrgAdmin {
	return &testOrgAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "Organization",
			SlugOverride:     "organizations",
			DisplayFields:    []string{"ID", "Name"},
			FormFieldNames:   []string{"Name"},
			SearchFieldNames: []string{"Name"},
			DeclaredFields:   []core.Field{core.NewField("Name", core.FieldTypeString)},
		},
		store: make(map[int]*testOrg),
	}
}

func (a *testOrgAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, o := range a.store {
		out = append(out, o)
	}
	return out, nil
}

func (a *testOrgAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	o, ok := a.store[id]
	if !ok {
		return nil, nil
	}
	return o, nil
}

type relUser struct {
	ID           int
	Email        string
	Organization *testOrg
	// []any, not []*testOrg: the adapter reads a many-to-many's current
	// value with current.([]any), the same "collections are []any"
	// convention its GetQueryset note spells out. A typed slice here
	// asserts to nothing and the field renders as an empty selection.
	Teams []any
}

var orgRelation = core.Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}

// Teams reuses the organizations admin as its target -- the widget only
// cares that a relation resolves to (pk, label) pairs, not what the
// target models.
var teamsRelation = core.Relation{Name: "Teams", Target: "organizations", DisplayField: "Name"}

type relUserAdmin struct {
	core.BaseModelAdmin
	store  map[int]*relUser
	nextID int
}

func newRelUserAdmin() *relUserAdmin {
	return &relUserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "Organization"},
			DetailFieldNames: []string{"ID", "Email", "Organization", "Teams"},
			FormFieldNames:   []string{"Email", "Organization", "Teams"},
			DeclaredFields: []core.Field{
				core.NewField("Email", core.FieldTypeEmail),
				core.NewField("Organization", core.FieldTypeForeignKey, core.WithRelation(orgRelation)),
				core.NewField("Teams", core.FieldTypeManyToMany, core.WithRelation(teamsRelation)),
			},
		},
		store:  make(map[int]*relUser),
		nextID: 1,
	}
}

func (a *relUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, 0, len(a.store))
	for _, u := range a.store {
		out = append(out, u)
	}
	return out, nil
}

func (a *relUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
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

func makeRelationApp(t *testing.T) (*fiber.App, *relUserAdmin, *testOrgAdmin) {
	t.Helper()
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)
	return app, userAdmin, orgAdmin
}

func TestListRendersRelatedLink(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users", nil)
	text := body(t, resp)
	if !strings.Contains(text, `href="/admin/organizations/1"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, ">Acme<") {
		t.Fatalf("got %s", text)
	}
}

func TestListShowsDashWhenRelationIsNone(t *testing.T) {
	app, userAdmin, _ := makeRelationApp(t)
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com"}

	resp := doGet(t, app, "/admin/users", nil)
	// The em dash itself, not the &mdash; entity: the placeholder is now
	// a configurable string (EmptyValueDisplay) written through
	// html.EscapeString, which leaves a literal em dash alone.
	if !strings.Contains(body(t, resp), core.DefaultEmptyValue) {
		t.Fatalf("expected a dash for the empty relation")
	}
}

func TestDetailRendersRelatedLink(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1", nil)
	text := body(t, resp)
	if !strings.Contains(text, `href="/admin/organizations/1"`) || !strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
}

func TestCreateFormShowsRelationSelectWithOptions(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}

	resp := doGet(t, app, "/admin/users/create", nil)
	text := body(t, resp)
	if !strings.Contains(text, `name="Organization"`) || !strings.Contains(text, ">Acme<") {
		t.Fatalf("got %s", text)
	}
}

func TestEditFormPreselectsCurrentRelation(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1/edit", nil)
	text := body(t, resp)
	// The plain (non-autocomplete) relation field is now the shadcn
	// Select port (ui/select.html): a hidden input carries the pk, and
	// the trigger's initial label text is the target's own label, not
	// an <option>.
	if !strings.Contains(text, `value="1"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, "Acme") {
		t.Fatalf("expected the current relation's label as the trigger text, got %s", text)
	}
}

func TestLookupRouteReturnsMatchingOptions(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[2] = &testOrg{ID: 2, Name: "Widgets Inc"}

	resp := doGet(t, app, "/admin/organizations/lookup?q=acme", nil)
	text := body(t, resp)
	if resp.StatusCode != 200 {
		t.Fatalf("got %d", resp.StatusCode)
	}
	if !strings.Contains(text, `data-pk="1"`) || !strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
	if strings.Contains(text, "Widgets Inc") {
		t.Fatalf("got %s", text)
	}
}

// multiSelectMarkup returns just the multi-select component's markup:
// from its x-data to the next component's, so an assertion about this
// control can neither be satisfied nor broken by a sibling field.
func multiSelectMarkup(t *testing.T, page string) string {
	t.Helper()
	start := strings.Index(page, `x-data="adminMultiSelect()"`)
	if start < 0 {
		t.Fatal("no multi-select on the page")
	}
	rest := page[start+len(`x-data="adminMultiSelect()"`):]
	if end := strings.Index(rest, `x-data=`); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func TestManyToManyRendersSearchableMultiSelectNotANativeMultiple(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[2] = &testOrg{ID: 2, Name: "Widgets Inc"}

	text := body(t, doGet(t, app, "/admin/users/create", nil))

	if strings.Contains(text, "<select multiple") {
		t.Error("expected the native <select multiple> to be gone")
	}
	if !strings.Contains(text, `x-data="adminMultiSelect()"`) {
		t.Error("expected the multi-select component")
	}
	// The list is "what you can still add": a chosen option leaves it
	// for a chip, so nothing in it is ever in a selected state -- no
	// check indicator, and no aria-selected to carry.
	if !strings.Contains(text, `x-show="available($el)"`) {
		t.Error("expected the list to hide options once they are chosen")
	}
	// Scoped to the multi-select's own markup: this form also renders a
	// ui/select for the foreign key, and that one carries aria-selected
	// legitimately (its list does show a chosen option).
	if ms := multiSelectMarkup(t, text); strings.Contains(ms, "aria-selected") ||
		strings.Contains(ms, "aria-multiselectable") {
		t.Error("expected no selected-state ARIA on a list that never shows selected options")
	}
	// Every option is in the page -- a many-to-many's list is already
	// fully rendered, which is what lets the search filter client-side.
	for _, want := range []string{`data-value="1"`, `data-value="2"`, "Acme", "Widgets Inc"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected option %q in the page", want)
		}
	}
	if !strings.Contains(text, `placeholder="Search…"`) {
		t.Error("expected the search box that makes a long list usable")
	}
}

// The chip's "Remove <label>" is formatted on the server, so a
// translation that reorders its verb (%[1]s) still places the label:
// Alpine only swaps the {label} marker Sprintf put there for the chip's
// own label, read off the element rather than quoted into the expression.
func TestMultiSelectRemoveLabelIsFormattedOnTheServer(t *testing.T) {
	catalog := fstest.MapFS{"fr.json": &fstest.MapFile{Data: []byte(`{"Remove %s": "Retirer %[1]s"}`)}}
	userAdmin, orgAdmin := newRelUserAdmin(), newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	app := newTestApp(t, core.New(core.WithModelAdmins(userAdmin, orgAdmin), core.WithCatalogs(catalog)))

	for _, tc := range []struct{ lang, want string }{
		{"en", `data-remove-label="Remove {label}"`},
		{"fr", `data-remove-label="Retirer {label}"`},
	} {
		ms := multiSelectMarkup(t, body(t, doGet(t, app, "/admin/users/create", map[string]string{"Accept-Language": tc.lang})))
		if !strings.Contains(ms, tc.want) {
			t.Errorf("%s: want %s in %s", tc.lang, tc.want, ms)
		}
		if want := `:aria-label="$el.dataset.removeLabel.replace('{label}', () => item.label)"`; !strings.Contains(ms, want) {
			t.Errorf("%s: want %s in %s", tc.lang, want, ms)
		}
	}
}

func TestManyToManySelectionPostsUnderTheFieldName(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	acme := &testOrg{ID: 1, Name: "Acme"}
	widgets := &testOrg{ID: 2, Name: "Widgets Inc"}
	orgAdmin.store[1], orgAdmin.store[2] = acme, widgets
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Teams: []any{widgets}}

	text := body(t, doGet(t, app, "/admin/users/1/edit", nil))

	if !strings.Contains(text, `<input type="hidden" name="Teams"`) {
		t.Error("expected the hidden input template posting under the field name")
	}
	// The current selection is marked on the option, which is what the
	// component hydrates its initial state from.
	if !strings.Contains(text, `data-value="2" data-label="Widgets Inc" data-selected="true"`) {
		t.Errorf("expected the chosen option pre-marked, got %s", text)
	}
	if strings.Contains(text, `data-value="1" data-label="Acme" data-selected="true"`) {
		t.Error("expected an unchosen option not to be marked selected")
	}
}

type denyOrgViewAuthorizer struct{}

func (denyOrgViewAuthorizer) Can(principal *core.Principal, permission string, resource any) bool {
	return permission != "organizations.view"
}

func TestLookupRouteRespectsAuthorization(t *testing.T) {
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin), core.WithAuthorizer(denyOrgViewAuthorizer{}))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/organizations/lookup?q=acme", nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("got %d", resp.StatusCode)
	}
}

type autocompleteRelUserAdmin struct {
	relUserAdmin
}

func newAutocompleteRelUserAdmin() *autocompleteRelUserAdmin {
	a := &autocompleteRelUserAdmin{relUserAdmin: *newRelUserAdmin()}
	a.AutocompleteFieldNames = []string{"Organization"}
	// Drop the many-to-many: it targets the same admin and renders every
	// option inline (that is what a multi-select is), which would defeat
	// the "an autocomplete field never dumps its target's queryset into
	// the page" assertion these fixtures exist to make.
	a.FormFieldNames = []string{"Email", "Organization"}
	return a
}

func TestAutocompleteFieldRendersCommandNotSelect(t *testing.T) {
	userAdmin := newAutocompleteRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/users/create", nil)
	text := body(t, resp)
	if !strings.Contains(text, "selectItem(") {
		t.Fatalf("expected the command widget's selectItem call, got %s", text)
	}
	if !strings.Contains(text, `id="combobox-results-Organization"`) {
		t.Fatalf("got %s", text)
	}
	if strings.Contains(text, `<select id="field-Organization"`) {
		t.Fatalf("expected no plain <select> for an autocomplete field, got %s", text)
	}
	// The target's full queryset must not be dumped into the page --
	// that's the whole point of routing this field through /lookup.
	if strings.Contains(text, "Acme") {
		t.Fatalf("got %s", text)
	}
}

func TestAutocompleteFieldPrefillsSelectionOnEdit(t *testing.T) {
	userAdmin := newAutocompleteRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	resp := doGet(t, app, "/admin/users/1/edit", nil)
	text := body(t, resp)
	if !strings.Contains(text, `value="Acme"`) {
		t.Fatalf("got %s", text)
	}
	if !strings.Contains(text, `value="1"`) {
		t.Fatalf("got %s", text)
	}
}

func TestRelatedLinkHiddenWhenTargetNotViewable(t *testing.T) {
	userAdmin := newRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	orgAdmin.BaseModelAdmin.DisableView = true
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	acme := &testOrg{ID: 1, Name: "Acme"}
	orgAdmin.store[1] = acme
	userAdmin.store[1] = &relUser{ID: 1, Email: "john@example.com", Organization: acme}

	resp := doGet(t, app, "/admin/users/1", nil)
	text := body(t, resp)
	if strings.Contains(text, `href="/admin/organizations/1"`) {
		t.Fatalf("expected no link, got %s", text)
	}
	if !strings.Contains(text, "Acme") {
		t.Fatalf("expected plain-text label still shown, got %s", text)
	}
}

func TestMultiSelectIsKeyboardOperable(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	ms := multiSelectMarkup(t, body(t, doGet(t, app, "/admin/users/create", nil)))

	for _, want := range []string{
		`@keydown.down.prevent="move(1)"`,
		`@keydown.up.prevent="move(-1)"`,
		`@keydown.enter.prevent="chooseActive()"`,
		`:aria-activedescendant="activeId || null"`,
		`role="combobox"`,
	} {
		if !strings.Contains(ms, want) {
			t.Errorf("multi-select is missing %q", want)
		}
	}
	if strings.Contains(ms, `role="option" tabindex="0"`) {
		t.Error("options must not be individually tabbable -- the search box holds focus")
	}
}

func TestFilterDrawerTrapsFocus(t *testing.T) {
	page := filterablePage(t, nil)
	if !strings.Contains(page, `x-trap="open"`) {
		t.Error("the filter drawer declares aria-modal but does not trap focus")
	}
}

func TestRelationComboboxIsAnnouncedToAssistiveTech(t *testing.T) {
	// newAutocompleteRelUserAdmin, not makeRelationApp: the plain
	// fixture renders a ui/select for the relation and a multi-select
	// for the m2m, both of which carry these same attributes -- the
	// assertions below would pass without the combobox being on the
	// page at all. This fixture drops the m2m and is the only one that
	// renders the component under test.
	userAdmin := newAutocompleteRelUserAdmin()
	orgAdmin := newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin))
	app := newTestApp(t, admin)

	page := body(t, doGet(t, app, "/admin/users/create", nil))
	if strings.Contains(page, "adminMultiSelect()") || strings.Contains(page, "adminSelect()") {
		t.Fatal("fixture leaked another listbox onto the page; these assertions would be vacuous")
	}

	for _, want := range []string{
		`role="combobox"`,
		`aria-autocomplete="list"`,
		`:aria-activedescendant="activeId || null"`,
		`role="listbox"`,
		// A swap replaces the results, so the remembered highlight has
		// to be dropped or activeId names a detached node.
		`@htmx:after-swap="clearActive()"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("relation combobox is missing %q", want)
		}
	}
}

func TestLookupResultsAreListboxOptions(t *testing.T) {
	app, _, orgAdmin := makeRelationApp(t)
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}

	fragment := body(t, doGet(t, app, "/admin/organizations/lookup?q=acme", nil))
	if !strings.Contains(fragment, `role="option"`) {
		t.Error("lookup results must be options, or the panel is a listbox with no options in it")
	}
}

func TestARelationFilterListsTheTargetsRecords(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t)
	userAdmin.DeclaredFilters = []core.Filter{core.NewRelationFilter("Organization")}
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}

	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, "Acme") {
		t.Error("the relation filter does not offer the target's records")
	}
	if !strings.Contains(page, "filter[Organization]=1") {
		t.Error("a relation choice does not carry the target's primary key")
	}
}

func TestARelationFilterDisappearsWhenTheTargetIsNotViewable(t *testing.T) {
	// Not rendered empty: an empty group is a control that looks broken.
	// A reader who may not see organizations is not told they exist.
	userAdmin := newRelUserAdmin()
	userAdmin.DeclaredFilters = []core.Filter{core.NewRelationFilter("Organization")}
	orgAdmin := newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	admin := core.New(core.WithModelAdmins(userAdmin, orgAdmin),
		core.WithAuthorizer(denyOrgViewAuthorizer{}))
	app := newTestApp(t, admin)

	page := body(t, doGet(t, app, "/admin/users", nil))
	if strings.Contains(page, "filter[Organization]") {
		t.Error("a filter over a target the principal cannot view was offered")
	}
}

func TestALargeRelationFilterUsesTheCombobox(t *testing.T) {
	// newAutocompleteRelUserAdmin already declares Organization in
	// AutocompleteFieldNames -- it is the fixture the form's combobox
	// tests use, which is the point: one declaration, both controls.
	userAdmin := newAutocompleteRelUserAdmin()
	userAdmin.DeclaredFilters = []core.Filter{core.NewRelationFilter("Organization")}
	orgAdmin := newTestOrgAdmin()
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}
	app := newTestApp(t, core.New(core.WithModelAdmins(userAdmin, orgAdmin)))

	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, `name="filter[Organization]"`) {
		t.Fatal("the panel did not render a combobox input for an autocomplete relation")
	}
	// It queries the target's own lookup route, which is already
	// authorized against that target's view permission.
	if !strings.Contains(page, "/admin/organizations/lookup") {
		t.Error("the panel's combobox does not point at the target's lookup route")
	}
	// The whole queryset must NOT have been dumped into the panel.
	if strings.Contains(page, `filter[Organization]=1`) {
		t.Error("the combobox branch still rendered a link list")
	}
}

func TestASmallRelationFilterStaysALinkList(t *testing.T) {
	app, userAdmin, orgAdmin := makeRelationApp(t) // no AutocompleteFieldNames
	userAdmin.DeclaredFilters = []core.Filter{core.NewRelationFilter("Organization")}
	orgAdmin.store[1] = &testOrg{ID: 1, Name: "Acme"}

	page := body(t, doGet(t, app, "/admin/users", nil))
	if !strings.Contains(page, "filter[Organization]=1") {
		t.Error("a relation not in AutocompleteFields should render as links")
	}
}
