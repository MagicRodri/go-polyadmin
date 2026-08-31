package fiber

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"
)

// -- date picker (Phase D) -----------------------------------------------

type datedThing struct {
	ID       int
	Name     string
	DueDate  string
	Priority string
}

type datedAdmin struct {
	core.BaseModelAdmin
	item *datedThing
}

func newDatedAdmin() *datedAdmin {
	return &datedAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:      "Task",
			DisplayFields:  []string{"ID", "Name", "DueDate", "Priority"},
			FormFieldNames: []string{"Name", "DueDate", "Priority"},
			DeclaredFields: []core.Field{
				core.NewField("Name", core.FieldTypeString, core.WithRequired()),
				core.NewField("DueDate", core.FieldTypeDate),
				core.NewField("Priority", core.FieldTypeEnum, core.WithChoices("Low", "Medium", "High")),
			},
		},
		item: &datedThing{ID: 1, Name: "Ship it", DueDate: "2026-03-14", Priority: "Medium"},
	}
}

func (a *datedAdmin) GetQueryset(ctx context.Context) (any, error) {
	return []any{a.item}, nil
}

func (a *datedAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	return a.item, nil
}

func (a *datedAdmin) GetPK(obj any) any { return obj.(*datedThing).ID }

func datedFormPage(t *testing.T, path string) string {
	t.Helper()
	admin := core.New(core.WithModelAdmins(newDatedAdmin()))
	app := newTestApp(t, admin)
	return body(t, doGet(t, app, path, nil))
}

func TestDateFieldRendersNativeInputPlusCalendarPopover(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/create")

	// The native input is what actually posts, and is what keeps the
	// field working with Alpine absent -- it must survive the
	// enhancement, not be replaced by it.
	if !strings.Contains(page, `type="date" id="field-DueDate" name="DueDate"`) {
		t.Error("expected the native date input to remain the posting control")
	}
	// The Calendar port is layered on top.
	for _, want := range []string{
		`x-data="adminCalendar()"`,
		`x-ref="dateInput"`,
		`aria-label="Open calendar"`,
		`x-anchor.bottom-end.offset.6="$refs.trigger"`,
		`x-for="day in days"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("date picker missing %q", want)
		}
	}
}

func TestDateFieldPrefillsExistingValue(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/1/edit")
	if !strings.Contains(page, `value="2026-03-14"`) {
		t.Error("expected the stored date to be prefilled on the native input")
	}
}

func TestCalendarFactoryIsDefinedOncePerPage(t *testing.T) {
	// The factory guards itself with `window.adminCalendar ||`, but the
	// <script> should still only be emitted once per page even when
	// several date fields are present -- it comes from base.html, not
	// from the field.
	page := datedFormPage(t, "/admin/tasks/create")
	if got := strings.Count(page, "window.adminCalendar = window.adminCalendar ||"); got != 1 {
		t.Errorf("expected the calendar factory once, got %d", got)
	}
}

func TestDateFieldStillWrappedInTheFormFieldUnit(t *testing.T) {
	// The picker replaces the *control*, not the label/description/error
	// wrapper the other field types share.
	page := datedFormPage(t, "/admin/tasks/create")
	if !strings.Contains(page, `<label for="field-DueDate"`) {
		t.Error("date field lost its label")
	}
}

// Filtering is one drawer behind one toolbar trigger (Django admin's
// filter column, in Unfold's drawer form), not a dropdown per filter --
// so a ModelAdmin's filter count costs the toolbar nothing.
func TestFiltersRenderAsOneDrawerBehindOneTrigger(t *testing.T) {
	page := filterablePage(t, nil)

	filters, err := uiClasses("toolbar", "filters")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(page, filters) {
		t.Error("expected a toolbar filter cluster")
	}
	if !strings.Contains(page, `aria-haspopup="dialog"`) {
		t.Error("expected one Filters trigger opening a drawer")
	}
	if !strings.Contains(page, `aria-label="Filters"`) {
		t.Error("expected the drawer itself")
	}
	// The filter's own label and choices live in the drawer body.
	if !strings.Contains(page, "Is Active") {
		t.Error("expected the filter's label in the drawer")
	}
	sideRight, err := uiClasses("sheet", "side-right")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(page, sideRight) {
		t.Error("expected the drawer to come in from the right")
	}
}

// The trigger carries a count so the drawer says how much it's hiding
// without being opened -- and only once something is applied.
func TestFilterTriggerCountsOnlyAppliedFilters(t *testing.T) {
	count, err := uiClasses("filter-panel", "count")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.Contains(filterablePage(t, nil), count) {
		t.Error("expected no count badge while nothing is filtered")
	}
	applied := filterablePage(t, map[string]string{"filter[IsActive]": "true"})
	if !strings.Contains(applied, count) {
		t.Error("expected a count badge once a filter is applied")
	}
}

// Reset clears search *and* every filter, so it lives with the things
// it clears -- in the drawer's footer -- which is also what keeps the
// stacked mobile toolbar to its five controls.
func TestResetLivesInTheDrawerAndOnlyWhenSomethingIsApplied(t *testing.T) {
	if strings.Contains(filterablePage(t, nil), "Clear all") {
		t.Error("expected no Reset while nothing is applied")
	}
	applied := filterablePage(t, map[string]string{"filter[IsActive]": "true"})
	if !strings.Contains(applied, "Clear all") {
		t.Error("expected Reset in the drawer once a filter is applied")
	}
}

// Every toolbar control fills its own line while the toolbar is a
// single stacked column below sm. The ones wrapped in a <form> or a
// positioning <div> can't inherit that from the flex row, so each
// carries "toolbar item" itself.
func TestToolbarControlsFillTheirLineWhileStacked(t *testing.T) {
	item, err := uiClasses("toolbar", "item")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	page := filterablePage(t, nil)
	// search, the Filters trigger (wrapper + button), bulk actions
	// (form + button), Export, New.
	if got := strings.Count(page, item); got < 6 {
		t.Errorf("expected every stacked toolbar control to fill its line, found %d occurrences of %q", got, item)
	}
}

// filterablePage renders the list view of a ModelAdmin that declares a
// filter and has actions/export/create available, with `query` applied.
func filterablePage(t *testing.T, query map[string]string) string {
	t.Helper()
	filterable := newActionableUserAdmin()
	filterable.DeclaredFilters = []core.Filter{core.NewBooleanFilter("IsActive")}
	filterable.createUser("jane@example.com", true)
	admin := core.New(core.WithModelAdmins(filterable))
	app := newTestApp(t, admin)

	path := "/admin/users"
	if len(query) > 0 {
		parts := make([]string, 0, len(query))
		for k, v := range query {
			parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
		path += "?" + strings.Join(parts, "&")
	}
	return body(t, doGet(t, app, path, nil))
}

// A stacked toolbar control is a full-width bar, so its label goes hard
// left and its icon hard right rather than sitting centred. Every
// control that has both carries the pair.
func TestStackedToolbarControlsPutTheLabelLeftAndTheIconRight(t *testing.T) {
	label, err := uiClasses("toolbar", "item-label")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	iconClass, err := uiClasses("toolbar", "item-icon")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	page := filterablePage(t, nil)

	// Filters, the action select, Export and New each get a label that
	// takes the slack.
	if got := strings.Count(page, label); got < 4 {
		t.Errorf("expected each stacked control's label to take the slack, found %d occurrences", got)
	}
	// Filters' and New's leading icons plus Export's icon+chevron move
	// to the trailing edge; the action select's chevron is already last
	// and needs no reorder.
	if got := strings.Count(page, iconClass); got < 4 {
		t.Errorf("expected leading icons to move to the trailing edge, found %d occurrences", got)
	}
}

// A label is arbitrary application text, so it reaches Alpine as a data
// attribute the browser decodes -- never quoted into the x-data
// expression, where one stray quote closes the attribute and every
// select on the page fails to initialise. (The Python mirror shipped
// exactly that bug via tojson.)
func TestSelectLabelIsADataAttributeNotAJSStringLiteral(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/1/edit")

	if !strings.Contains(page, `x-data="{ open: false, label: '' }"`) {
		t.Error("expected the x-data to carry no interpolated label")
	}
	if !strings.Contains(page, `x-init="label = $el.dataset.label"`) {
		t.Error("expected the label to be hydrated from the DOM")
	}
	if !strings.Contains(page, `data-label="Medium"`) {
		t.Error("expected the current value's label as a data attribute")
	}
}

// -- booleans as icons ----------------------------------------------------

// A column of booleans is scannable as glyphs and not as two
// similar-length words, so list cells render a check or a cross. The
// word stays as an sr-only label, so nothing depends on the icon alone.

// Exports stringify through core/exporter.go, never through
// fieldValueHTML, so a CSV still carries a readable value rather than
// an SVG.

// -- shadcn Select for plain choice fields -------------------------------

func TestEnumFieldRendersShadcnSelectNotNativeOptions(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/1/edit")
	if strings.Contains(page, "<option") {
		t.Error("expected no native <option> elements once enum uses ui/select")
	}
	if !strings.Contains(page, `aria-haspopup="listbox"`) {
		t.Error("expected the Select trigger")
	}
	if !strings.Contains(page, `name="Priority"`) {
		t.Error("expected a hidden input still posting the field under its own name")
	}
	if !strings.Contains(page, "Medium") {
		t.Error("expected the current value's label as the trigger text")
	}
}

func TestEnumFieldSelectListsAllChoicesAsOptions(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/create")
	for _, want := range []string{`data-value="Low"`, `data-value="Medium"`, `data-value="High"`} {
		if !strings.Contains(page, want) {
			t.Errorf("expected choice %q as a listbox option", want)
		}
	}
}

// -- export dropdown (Phase B) ------------------------------------------

func TestListRendersExportDropdownRatherThanOneButtonPerFormat(t *testing.T) {
	admin := core.New(core.WithModelAdmins(newTestUserAdmin()))
	app := newTestApp(t, admin)
	page := body(t, doGet(t, app, "/admin/users", nil))

	if !strings.Contains(page, `aria-haspopup="menu"`) {
		t.Error("expected the export DropdownMenu trigger")
	}
	if !strings.Contains(page, "/admin/users/export/csv") || !strings.Contains(page, "/admin/users/export/xlsx") {
		t.Error("both export formats should still be reachable as menu items")
	}
	if !strings.Contains(page, `role="menuitem"`) {
		t.Error("expected menu items")
	}
}

// -- list-view reordering ------------------------------------------------

func TestListShowsDragHandleOnlyWhenReorderable(t *testing.T) {
	plain := newTestUserAdmin()
	plain.createUser("jane@example.com", true)
	admin := core.New(core.WithModelAdmins(plain))
	app := newTestApp(t, admin)
	page := body(t, doGet(t, app, "/admin/users", nil))
	if strings.Contains(page, "drag-handle") {
		t.Error("expected no drag handle when EnableReordering is unset")
	}

	reorderable := newTestUserAdmin()
	reorderable.createUser("jane@example.com", true)
	reorderable.EnableReordering = true
	admin2 := core.New(core.WithModelAdmins(reorderable))
	app2 := newTestApp(t, admin2)
	page2 := body(t, doGet(t, app2, "/admin/users", nil))
	if !strings.Contains(page2, "drag-handle") {
		t.Error("expected a drag handle when EnableReordering is set")
	}
	if !strings.Contains(page2, "Sortable.create") {
		t.Error("expected the drag handle to be wired to SortableJS")
	}
}

func TestListRowActionsRenderAsOneDropdownMenu(t *testing.T) {
	userAdmin := newTestUserAdmin()
	userAdmin.createUser("jane@example.com", true)
	admin := core.New(core.WithModelAdmins(userAdmin))
	app := newTestApp(t, admin)
	page := body(t, doGet(t, app, "/admin/users", nil))

	if !strings.Contains(page, `aria-label="Open menu"`) {
		t.Error("expected a single row-actions menu trigger")
	}
	if !strings.Contains(page, `role="menuitem"`) {
		t.Error("expected View/Edit/Delete as menu items")
	}
	if strings.Contains(page, `title="View"`) || strings.Contains(page, `title="Edit"`) || strings.Contains(page, `title="Delete"`) {
		t.Error("row actions should no longer render as standalone icon buttons")
	}
}

// -- combobox (Phase D) --------------------------------------------------

func TestComboboxUsesTokenClasses(t *testing.T) {
	// The autocomplete relation field is the one genuinely Alpine-driven
	// form control; its panel/active-item classes must come from the
	// registry so it themes with everything else.
	got, err := uiClasses("combobox", "content")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if !strings.Contains(got, "bg-popover") {
		t.Errorf("combobox panel should use the popover token, got %q", got)
	}
	active, err := uiClasses("combobox", "item-active")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	// Applied/removed imperatively by the arrow-key handler, so it has
	// to stay a single class with no spaces.
	if strings.Contains(active, " ") {
		t.Errorf("item-active must be a single class (added via classList), got %q", active)
	}
}

// shadcn's ToastViewport, pinned bottom-right. pointer-events-none
// matters: record pages now carry a sticky action bar in that same
// corner, and the viewport spans a strip of the screen even with no
// toasts in it -- without it, Save would be unclickable.
func TestToastViewportSitsBottomRightAndDoesNotBlockClicks(t *testing.T) {
	viewport, err := uiClasses("toast", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"bottom-0", "sm:right-0", "pointer-events-none"} {
		if !strings.Contains(viewport, want) {
			t.Errorf("expected the toast viewport to carry %q", want)
		}
	}
	if strings.Contains(viewport, "top-4") {
		t.Error("toasts should no longer be top-anchored")
	}
	root, _ := uiClasses("toast", "root")
	if !strings.Contains(root, "pointer-events-auto") {
		t.Error("each toast must re-enable pointer events for itself")
	}

	ua := newTestUserAdmin()
	ua.createUser("a@example.com", true)
	app := newTestApp(t, core.New(core.WithModelAdmins(ua)))
	if !strings.Contains(body(t, doGet(t, app, "/admin/users", nil)), viewport) {
		t.Error("expected the toast viewport on the page")
	}
}
