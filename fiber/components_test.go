package fiber

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
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

	if !strings.Contains(page, `x-data="adminSelect()"`) {
		t.Error("expected the x-data to be the bare factory call, with no interpolated label")
	}
	if !strings.Contains(page, `x-init="hydrate()"`) {
		t.Error("expected the label to be hydrated from the DOM")
	}
	// The point of the whole arrangement: the label must never appear
	// inside the Alpine expression, only as an HTML attribute value.
	if strings.Contains(page, `label: 'Medium'`) || strings.Contains(page, `label: "Medium"`) {
		t.Error("the label must not be interpolated into a JS string literal")
	}
	if !strings.Contains(page, `data-label="Medium"`) {
		t.Error("expected the current value's label as a data attribute")
	}
}

// -- booleans as icons ----------------------------------------------------

// A column of booleans is scannable as glyphs and not as two
// similar-length words, so list cells render a check or a cross. The
// word stays as an sr-only label, so nothing depends on the icon alone.
func TestBooleanCellsRenderAsIconsWithAnAccessibleLabel(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("yes@example.com", true)
	userAdmin.createUser("no@example.com", false)

	page := body(t, doGet(t, app, "/admin/users", nil))

	if !strings.Contains(page, string(iconHTML("check", "size-4"))) {
		t.Error("expected a check icon for a true boolean")
	}
	if !strings.Contains(page, string(iconHTML("close", "size-4"))) {
		t.Error("expected a cross icon for a false boolean")
	}
	for _, want := range []string{`<span class="sr-only">Yes</span>`, `<span class="sr-only">No</span>`} {
		if !strings.Contains(page, want) {
			t.Errorf("expected %q so the value is not conveyed by the icon alone", want)
		}
	}
	// The old plain-text rendering is gone: the words survive only as
	// the sr-only labels asserted above.
	if strings.Contains(page, `dark:text-emerald-400">Yes<`) {
		t.Error("expected the bare Yes/No text rendering to be gone")
	}
}

// Exports stringify through core/exporter.go, never through
// fieldValueHTML, so a CSV still carries a readable value rather than
// an SVG.
func TestBooleanExportIsUnaffectedByTheIconRendering(t *testing.T) {
	app, userAdmin := makeApp(t)
	userAdmin.createUser("yes@example.com", true)

	csv := body(t, doGet(t, app, "/admin/users/export/csv", nil))
	if strings.Contains(csv, "<svg") || strings.Contains(csv, "sr-only") {
		t.Errorf("export leaked list markup: %s", csv)
	}
	if !strings.Contains(csv, "true") {
		t.Errorf("expected the boolean as text in the export, got %s", csv)
	}
}

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

// The three listbox-ish components declare ARIA roles, which is a
// promise to assistive tech that the keyboard works. These pin the
// mechanics that make the promise true; the behaviour itself is
// exercised in a browser (see the accessibility CDP run).
func TestSelectIsKeyboardOperable(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/1/edit")

	for _, want := range []string{
		`@keydown.down.prevent="openAndMove(1)"`,
		`@keydown.up.prevent="openAndMove(-1)"`,
		`@keydown.home.prevent="if (open) setActive(optionEls()[0])"`,
		// Focus stays on the trigger, so the trigger is what names the
		// highlighted option.
		`:aria-activedescendant="open ? activeId : null"`,
		`:aria-controls="$id('select-listbox')"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("select is missing %q", want)
		}
	}
	// tabindex="0" on every option would make Tab walk the whole list;
	// with a roving highlight the options must be out of the tab order.
	if strings.Contains(page, `role="option" tabindex="0"`) {
		t.Error("options must not be individually tabbable")
	}
}

// -- fieldsets ------------------------------------------------------------

type fieldsetAdmin struct{ datedAdmin }

func newFieldsetAdmin() *fieldsetAdmin {
	a := &fieldsetAdmin{datedAdmin: *newDatedAdmin()}
	a.DeclaredFieldsets = []core.Fieldset{
		{Fields: []string{"Name"}},
		{Title: "Scheduling", Description: "When it is due.", Fields: []string{"DueDate"}},
		{Title: "Advanced", Fields: []string{"Priority"}, Collapsed: true},
	}
	return a
}

func fieldsetFormPage(t *testing.T) string {
	t.Helper()
	admin := core.New(core.WithModelAdmins(newFieldsetAdmin()))
	return body(t, doGet(t, newTestApp(t, admin), "/admin/tasks/1/edit", nil))
}

func TestDeclaredFieldsetsRenderAsTitledGroups(t *testing.T) {
	page := fieldsetFormPage(t)
	for _, want := range []string{"Scheduling", "When it is due.", "Advanced"} {
		if !strings.Contains(page, want) {
			t.Errorf("expected %q in the form", want)
		}
	}
	// Every field still renders -- grouping must not drop any.
	for _, name := range []string{`name="Name"`, `name="DueDate"`, `name="Priority"`} {
		if !strings.Contains(page, name) {
			t.Errorf("field %s went missing when grouped", name)
		}
	}
}

func TestCollapsedFieldsetStartsClosedAndOthersOpen(t *testing.T) {
	page := fieldsetFormPage(t)
	if !strings.Contains(page, `x-data="{ open: false }"`) {
		t.Error("expected the Collapsed group to start closed")
	}
	if !strings.Contains(page, `x-data="{ open: true }"`) {
		t.Error("expected the uncollapsed titled group to start open")
	}
}

// The default case must not gain a wrapper: an admin that declares no
// fieldsets should render exactly the flat form it always did.
func TestUndeclaredFieldsetsRenderNoGroupChrome(t *testing.T) {
	page := datedFormPage(t, "/admin/tasks/1/edit")
	base, err := uiClasses("fieldset")
	if err != nil {
		t.Fatalf("uiClasses: %v", err)
	}
	if strings.Contains(page, base) {
		t.Error("a form with no declared fieldsets must render no fieldset chrome")
	}
	if !strings.Contains(page, `name="Name"`) {
		t.Error("the flat form still has to render its fields")
	}
}

// -- read-only fields -----------------------------------------------------

type readOnlyAdmin struct{ datedAdmin }

func newReadOnlyAdmin() *readOnlyAdmin {
	a := &readOnlyAdmin{datedAdmin: *newDatedAdmin()}
	a.ReadOnlyFieldNames = []string{"Name"}
	return a
}

func (a *readOnlyAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	item := obj.(*datedThing)
	// Deliberately writes whatever it is handed: the protection has to
	// come from the framework not passing the value, not from the
	// application remembering to ignore it.
	if v, ok := data["Name"].(string); ok {
		item.Name = v
	}
	if v, ok := data["Priority"].(string); ok {
		item.Priority = v
	}
	return item, nil
}

func readOnlyApp(t *testing.T) (*fiber.App, *readOnlyAdmin) {
	t.Helper()
	ma := newReadOnlyAdmin()
	return newTestApp(t, core.New(core.WithModelAdmins(ma))), ma
}

// The one that matters: omitting the input is presentation, not
// enforcement. A crafted POST must not be able to write the field.
func TestReadOnlyFieldIsRefusedEvenWhenPosted(t *testing.T) {
	app, ma := readOnlyApp(t)
	before := ma.item.Name

	resp := doPostForm(t, app, "/admin/tasks/1/edit", url.Values{
		"Name":     {"hacked"},
		"DueDate":  {"2026-03-14"},
		"Priority": {"High"},
	}, nil)
	if resp.StatusCode >= 400 {
		t.Fatalf("expected the save to succeed, got %d", resp.StatusCode)
	}
	if ma.item.Name != before {
		t.Errorf("a read-only field was written by a posted value: %q -> %q", before, ma.item.Name)
	}
	// The writable field on the same form must still save, or this is
	// just a broken form rather than a protected field.
	if ma.item.Priority != "High" {
		t.Errorf("the writable field did not save: %q", ma.item.Priority)
	}
}

func TestReadOnlyFieldRendersAsAValueNotAnInput(t *testing.T) {
	app, _ := readOnlyApp(t)
	page := body(t, doGet(t, app, "/admin/tasks/1/edit", nil))

	if strings.Contains(page, `name="Name"`) {
		t.Error("a read-only field must not render a posting input")
	}
	if !strings.Contains(page, "Ship it") {
		t.Error("expected the read-only field's value to still be shown")
	}
	// The writable fields are untouched.
	if !strings.Contains(page, `name="Priority"`) {
		t.Error("writable fields must still render inputs")
	}
}
