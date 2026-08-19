package fiber

import (
	"context"
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

// Filters are faceted dropdowns in the toolbar now (shadcn's Tasks
// example), not a side panel -- so they sit inside the toolbar's
// left-hand cluster, alongside search, rather than in a column of their
// own with mobile ordering to manage.
func TestFiltersRenderAsToolbarFacets(t *testing.T) {
	filterable := newTestUserAdmin()
	filterable.DeclaredFilters = []core.Filter{core.NewBooleanFilter("IsActive")}
	filterable.createUser("jane@example.com", true)
	admin := core.New(core.WithModelAdmins(filterable))
	app := newTestApp(t, admin)
	page := body(t, doGet(t, app, "/admin/users", nil))

	filters, _ := uiClasses("toolbar", "filters")
	if !strings.Contains(page, filters) {
		t.Error("expected a toolbar filter cluster")
	}
	facet, _ := uiClasses("toolbar", "facet")
	if !strings.Contains(page, facet) {
		t.Error("expected the filter to render as a dashed facet trigger")
	}
	if !strings.Contains(page, "Is Active") {
		t.Error("expected the facet trigger to be labelled with the filter")
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
