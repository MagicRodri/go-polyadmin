// Package fiber is the Fiber adapter: it mounts an
// *core.Admin as routes on a *fiber.App. Rendering here mirrors the
// Python FastAPI adapter's polyadmin/templating.py + polyadmin/core/template_context.py,
// but field/form HTML is built in Go (render_helpers.go) rather than
// inside the html/template files, for tighter control over escaping.
package fiber

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MagicRodri/go-polyadmin/core"
	coretemplates "github.com/MagicRodri/go-polyadmin/templates"
)

// navLink is one flat sidebar entry -- either top-level or nested
// inside a navGroup's Links. Icon is the ModelAdmin's/AdminPage's own
// icon, kept even when nested inside a group's accordion.
type navLink struct {
	Key    string
	Label  string
	URL    string
	Icon   string
	Active bool
}

// navGroupIcon is fixed, not configurable per category -- a category
// is just a string, not an object with its own settings -- so every
// accordion section uses the same icon, distinct from any resource's
// own icon (which nested links keep showing, see buildNav).
const navGroupIcon = "folder"

// navGroup is one category's accordion section -- ModelAdmins and
// AdminPages sharing a Category collapse into one of these, in
// first-registration-appearance order. See buildNav.
type navGroup struct {
	Label    string
	Icon     string
	Links    []navLink
	Expanded bool // seeds the accordion's initial Alpine `open` state
}

// navEntry is a poor-man's tagged union: html/template needs concrete
// fields to range/branch on, not an interface, so IsGroup selects
// which of Link/Group is populated.
type navEntry struct {
	IsGroup bool
	Link    navLink
	Group   navGroup
}

type permissions struct {
	CanView   bool
	CanCreate bool
	CanUpdate bool
	CanDelete bool
	CanExport bool
}

// breadcrumb is one entry in a page's trail (components rendered by
// base.html). URL == "" means not a link (either the category
// segment, which has no route of its own, or the current/active
// page); Active distinguishes the two -- only the active crumb gets
// the bold "current page" styling. base.html always prepends an
// implicit home-icon crumb linking to the dashboard, so breadcrumb
// lists here never include it.
type breadcrumb struct {
	Label  string
	URL    string
	Active bool
}

// categoryBreadcrumb is the category crumb, if any -- always the
// first segment after the implicit home crumb, never a link (there's
// no route for a category by itself) and never the active/current-page
// crumb.
func categoryBreadcrumb(category string) []breadcrumb {
	if category == "" {
		return nil
	}
	return []breadcrumb{{Label: category}}
}

type pageBase struct {
	// Principal is who is signed in, for the sidebar footer's NavUser
	// (shadcn sidebar-07). Every page renders a sidebar, so it has to
	// reach every page's data.
	Principal *core.Principal
	Title     string
	Header    string
	BasePath  string
	// CurrentSlug is the bare resource slug for a ModelAdmin view
	// ("" for the dashboard or a custom page) -- used directly by
	// form.html/delete.html to link back to the resource's list view.
	// CurrentNavKey is the sidebar-active-highlight key ("resource:"+
	// slug, "page:"+path, or "" for the dashboard); see buildNav.
	CurrentSlug   string
	CurrentNavKey string
	NavItems      []navEntry
	SiteTitle     string
	SiteLogoURL   string
	Breadcrumbs   []breadcrumb
	Messages      []flashMessage
}

// buildNav returns ordered sidebar entries: flat links and
// category-grouped accordion sections, interleaved in
// first-registration-appearance order across ModelAdmins and
// AdminPages -- mirrors the Python adapter's
// template_context.py:build_nav. ModelAdmins with CanView()==false and
// AdminPages with HideFromNav==true are omitted. A group's own
// Expanded flag (seeding the accordion's initial Alpine `open` state)
// is true iff any of its links is the current page.
func (r *Renderer) buildNav(activeKey string) []navEntry {
	var order []navEntry
	groupIndex := map[string]int{}

	add := func(link navLink, category string) {
		if category == "" {
			order = append(order, navEntry{Link: link})
			return
		}
		idx, ok := groupIndex[category]
		if !ok {
			order = append(order, navEntry{IsGroup: true, Group: navGroup{Label: category, Icon: navGroupIcon}})
			idx = len(order) - 1
			groupIndex[category] = idx
		}
		order[idx].Group.Links = append(order[idx].Group.Links, link)
	}

	for _, ma := range r.admin.ModelAdmins() {
		if ma.CanView() {
			key := "resource:" + ma.Slug()
			add(navLink{Key: key, Label: ma.VerboseName(), URL: r.basePath + "/" + ma.Slug(), Icon: ma.Icon(), Active: key == activeKey}, ma.Category())
		}
	}
	for _, p := range r.admin.Pages() {
		if !p.HideFromNav {
			key := "page:" + p.Path
			add(navLink{Key: key, Label: p.Label, URL: r.basePath + p.Path, Icon: p.Icon, Active: key == activeKey}, p.Category)
		}
	}

	for i := range order {
		if order[i].IsGroup {
			for _, l := range order[i].Group.Links {
				if l.Active {
					order[i].Group.Expanded = true
					break
				}
			}
		}
	}
	return order
}

func (r *Renderer) pageBase(principal *core.Principal, title, header, navKey string, breadcrumbs []breadcrumb, messages []flashMessage) pageBase {
	siteTitle := r.admin.SiteTitle
	if siteTitle == "" {
		siteTitle = "PolyAdmin"
	}
	var currentSlug string
	if slug, ok := strings.CutPrefix(navKey, "resource:"); ok {
		currentSlug = slug
	}
	return pageBase{
		Principal: principal,
		Title:     title, Header: header, BasePath: r.basePath,
		CurrentSlug: currentSlug, CurrentNavKey: navKey, NavItems: r.buildNav(navKey),
		SiteTitle: siteTitle, SiteLogoURL: r.admin.SiteLogoURL, Breadcrumbs: breadcrumbs, Messages: messages,
	}
}

// objectLabel is a short human-readable label for an object in a
// breadcrumb trail. Prefers the first SearchFields entry (usually the
// most identifying field, e.g. "email") over the first ListDisplay
// column (often the primary key), falling back to the primary key.
func objectLabel(modelAdmin core.ModelAdmin, obj any) string {
	names := modelAdmin.SearchFields()
	if len(names) == 0 {
		names = modelAdmin.ListDisplay()
	}
	if len(names) == 0 {
		names = modelAdmin.DetailFields()
	}
	if len(names) > 0 {
		if field, ok := modelAdmin.Field(names[0]); ok {
			return fmt.Sprint(field.GetValue(obj))
		}
	}
	return fmt.Sprint(modelAdmin.GetPK(obj))
}

func listBreadcrumbs(modelAdmin core.ModelAdmin) []breadcrumb {
	return append(categoryBreadcrumb(modelAdmin.Category()), breadcrumb{Label: modelAdmin.VerboseName(), Active: true})
}

func detailBreadcrumbs(modelAdmin core.ModelAdmin, obj any, basePath string) []breadcrumb {
	crumbs := categoryBreadcrumb(modelAdmin.Category())
	return append(crumbs,
		breadcrumb{Label: modelAdmin.VerboseName(), URL: fmt.Sprintf("%s/%s", basePath, modelAdmin.Slug())},
		breadcrumb{Label: objectLabel(modelAdmin, obj), Active: true},
	)
}

func formBreadcrumbs(modelAdmin core.ModelAdmin, obj any, basePath string) []breadcrumb {
	crumbs := categoryBreadcrumb(modelAdmin.Category())
	crumbs = append(crumbs, breadcrumb{Label: modelAdmin.VerboseName(), URL: fmt.Sprintf("%s/%s", basePath, modelAdmin.Slug())})
	if obj != nil {
		crumbs = append(crumbs,
			breadcrumb{Label: objectLabel(modelAdmin, obj), URL: fmt.Sprintf("%s/%s/%v", basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj))},
			breadcrumb{Label: "Edit", Active: true},
		)
	} else {
		crumbs = append(crumbs, breadcrumb{Label: "New", Active: true})
	}
	return crumbs
}

func deleteBreadcrumbs(modelAdmin core.ModelAdmin, obj any, basePath string) []breadcrumb {
	crumbs := categoryBreadcrumb(modelAdmin.Category())
	return append(crumbs,
		breadcrumb{Label: modelAdmin.VerboseName(), URL: fmt.Sprintf("%s/%s", basePath, modelAdmin.Slug())},
		breadcrumb{Label: objectLabel(modelAdmin, obj), URL: fmt.Sprintf("%s/%s/%v", basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj))},
		breadcrumb{Label: "Delete", Active: true},
	)
}

type Renderer struct {
	admin    *core.Admin
	basePath string

	list           *template.Template
	detail         *template.Template
	form           *template.Template
	deleteTpl      *template.Template
	dashboard      *template.Template
	widgets        *template.Template
	lookup         *template.Template
	inlineFragment *template.Template

	// uiSet holds just the shadcn-derived component partials, so Go
	// code that builds HTML directly (render_helpers.go's
	// formInputHTML) can still render one -- the date field's Calendar
	// popover -- instead of duplicating its markup as a Go string. See
	// uiHTML.
	uiSet *template.Template

	// templateDirs are application-supplied override directories,
	// searched in the order given before the framework's own embedded
	// templates -- see WithTemplateDirs and docs/templates.md. Empty
	// unless an application opts in.
	templateDirs []string

	// overrideCache holds per-(ModelAdmin slug, view, resolved file)
	// template sets built by contentTemplate, since -- unlike the
	// eagerly-built framework defaults above -- they're resolved lazily
	// on first use. A *template.Template is safe for concurrent
	// ExecuteTemplate calls once built, but building/caching it needs
	// its own lock since Fiber handlers run concurrently.
	overrideMu    sync.RWMutex
	overrideCache map[string]*template.Template
}

// layoutFiles are the framework templates every full-page template set
// needs: the layout itself, the theme's token/dark-mode block, and the
// two globally-teleported overlays. Kept as one list so adding a shared
// partial doesn't mean editing six ParseFS calls (and forgetting the
// override path in contentTemplate/PageTemplate).
var layoutFiles = []string{
	"admin/base.html",
	"admin/theme.html",
	"admin/toasts.html",
	"admin/action_confirm_modal.html",
}

// uiComponentsGlob matches the shadcn-derived component partials (see
// plan/shadcnui-usage.md §5). Parsed into every template set -- unlike
// the `ui` template func, which only yields a class string, these are
// whole markup+Alpine blocks (dialog, sheet, dropdown, tooltip, ...)
// invoked as {{template "ui/dialog" dict ...}}.
const uiComponentsGlob = "admin/components/ui/*.html"

// buildTemplate parses the shared layout + ui component partials plus
// the given content files into one set.
func buildTemplate(contentFiles ...string) (*template.Template, error) {
	files := append(append([]string{}, layoutFiles...), contentFiles...)
	tmpl, err := template.New(path.Base(files[0])).Funcs(templateFuncs).ParseFS(coretemplates.FS, files...)
	if err != nil {
		return nil, err
	}
	return tmpl.ParseFS(coretemplates.FS, uiComponentsGlob)
}

func NewRenderer(admin *core.Admin, basePath string, templateDirs ...string) (*Renderer, error) {
	r := &Renderer{admin: admin, basePath: basePath, templateDirs: templateDirs, overrideCache: make(map[string]*template.Template)}
	// Fragment-only sets (widgets, lookup, inline) render without the
	// base layout, so they take the ui partials and funcs but not
	// layoutFiles.
	buildFragment := func(files ...string) (*template.Template, error) {
		tmpl, err := template.New(path.Base(files[0])).Funcs(templateFuncs).ParseFS(coretemplates.FS, files...)
		if err != nil {
			return nil, err
		}
		return tmpl.ParseFS(coretemplates.FS, uiComponentsGlob)
	}
	var err error
	if r.list, err = buildTemplate("admin/list.html"); err != nil {
		return nil, err
	}
	if r.detail, err = buildTemplate("admin/inline.html", "admin/detail.html"); err != nil {
		return nil, err
	}
	if r.form, err = buildTemplate("admin/inline.html", "admin/form.html"); err != nil {
		return nil, err
	}
	if r.deleteTpl, err = buildTemplate("admin/delete.html"); err != nil {
		return nil, err
	}
	if r.dashboard, err = buildTemplate("admin/dashboard.html"); err != nil {
		return nil, err
	}
	if r.widgets, err = buildFragment("admin/widgets/*.html"); err != nil {
		return nil, err
	}
	if r.lookup, err = buildFragment("admin/lookup.html"); err != nil {
		return nil, err
	}
	if r.inlineFragment, err = buildFragment("admin/inline.html", "admin/inline_fragment.html"); err != nil {
		return nil, err
	}
	if r.uiSet, err = template.New("ui").Funcs(templateFuncs).ParseFS(coretemplates.FS, uiComponentsGlob); err != nil {
		return nil, err
	}
	return r, nil
}

// uiHTML renders one shadcn-derived component partial to HTML, for the
// Go-built markup in render_helpers.go. Returns template.HTML because
// the caller is assembling a larger HTML string by hand -- the partial's
// own output is already escaped by html/template.
func (r *Renderer) uiHTML(name string, data any) (template.HTML, error) {
	var buf bytes.Buffer
	if err := r.uiSet.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// templateCandidates mirrors the Python adapter's
// ModelAdmin.get_template_candidates(view): an explicit override,
// then a resource-specific template, then the framework default, in
// that priority order.
func templateCandidates(modelAdmin core.ModelAdmin, view string) []string {
	candidates := make([]string, 0, 3)
	if override := modelAdmin.TemplateOverride(view); override != "" {
		candidates = append(candidates, override)
	}
	candidates = append(candidates,
		"admin/resource/"+modelAdmin.Slug()+"/"+view+".html",
		"admin/"+view+".html",
	)
	return candidates
}

// resolveContentSource finds the first candidate that exists, checking
// the application's own template dirs (in the order given to
// WithTemplateDirs) before the framework's embedded templates --
// mirrors Jinja's FileSystemLoader search order in the Python adapter.
func (r *Renderer) resolveContentSource(candidates []string) (name string, source fs.FS, err error) {
	for _, candidate := range candidates {
		for _, dir := range r.templateDirs {
			if _, statErr := os.Stat(dir + "/" + candidate); statErr == nil {
				return candidate, os.DirFS(dir), nil
			}
		}
		if _, statErr := fs.Stat(coretemplates.FS, candidate); statErr == nil {
			return candidate, coretemplates.FS, nil
		}
	}
	return "", nil, fmt.Errorf("polyadmin: no template found among candidates %v", candidates)
}

// contentTemplate returns the *template.Template to render `view` for
// `modelAdmin` with: `fallback` (one of the framework-default sets
// built eagerly in NewRenderer) unless an explicit override or a
// resource-specific template file actually resolves to something else,
// in which case a fresh base+toasts+modal+content set is built (from
// whichever filesystem the winning candidate came from) and cached for
// reuse. Applications that never configure WithTemplateDirs or set a
// ModelAdmin's own *Template field pay no extra cost here -- they hit
// the fallback on the first check and never reach the cache at all.
func (r *Renderer) contentTemplate(modelAdmin core.ModelAdmin, view string, fallback *template.Template) (*template.Template, error) {
	if len(r.templateDirs) == 0 && modelAdmin.TemplateOverride(view) == "" {
		return fallback, nil
	}
	candidates := templateCandidates(modelAdmin, view)
	name, source, err := r.resolveContentSource(candidates)
	if err != nil {
		return nil, err
	}
	if source == fs.FS(coretemplates.FS) && name == "admin/"+view+".html" {
		// Resolved straight to the plain framework default -- no
		// override actually applies, so reuse the pre-built set.
		return fallback, nil
	}

	cacheKey := modelAdmin.Slug() + "|" + view + "|" + name
	r.overrideMu.RLock()
	cached, ok := r.overrideCache[cacheKey]
	r.overrideMu.RUnlock()
	if ok {
		return cached, nil
	}

	baseFiles := append([]string{}, layoutFiles...)
	if view == "form" || view == "detail" {
		// form/detail content may render inline sections via
		// {{template "inlineSection" .}} -- see admin/inline.html.
		baseFiles = append(baseFiles, "admin/inline.html")
	}
	tmpl, err := template.New(path.Base(name)).Funcs(templateFuncs).ParseFS(coretemplates.FS, baseFiles...)
	if err != nil {
		return nil, err
	}
	if tmpl, err = tmpl.ParseFS(coretemplates.FS, uiComponentsGlob); err != nil {
		return nil, err
	}
	if tmpl, err = tmpl.ParseFS(source, name); err != nil {
		return nil, err
	}

	r.overrideMu.Lock()
	r.overrideCache[cacheKey] = tmpl
	r.overrideMu.Unlock()
	return tmpl, nil
}

// -- list -------------------------------------------------------------

type columnHeader struct {
	Label     string
	NextSort  string
	Indicator string
	// Sort dropdown (shadcn Tasks example's DataTableColumnHeader):
	// explicit Asc/Desc choices rather than a link that cycles, so a
	// click's effect is knowable before making it. Direction is "asc",
	// "desc", or "" when this column isn't the one being sorted by.
	Direction string
	AscURL    string
	DescURL   string
}

// pageSizeOption is one choice in the footer's rows-per-page control.
type pageSizeOption struct {
	Size     int
	Selected bool
	URL      string
}

// pageURLs are the four jumps in the tasks-style pagination footer.
// Empty means the jump is unavailable from where we are.
type pageURLs struct {
	First    string
	Previous string
	Next     string
	Last     string
}

type listRow struct {
	PK    any
	Cells []template.HTML
}

type filterChoice struct {
	Value    string
	Label    string
	Selected bool
	URL      string
}

type filterControl struct {
	Name    string
	Label   string
	Choices []filterChoice
	// The toolbar renders each filter as a dropdown trigger, so it needs
	// the active choice's label for the trigger itself and a URL that
	// clears just this filter.
	Active   string
	ClearURL string
}

// actionInfo is an Action's template-facing shape --
// leaves out Handler/Permission since templates only need enough to
// render the action bar / record-action buttons and post to its route.
type actionInfo struct {
	Name    string
	Label   string
	Confirm string
}

func actionInfos(modelAdmin core.ModelAdmin) []actionInfo {
	actions := modelAdmin.Actions()
	out := make([]actionInfo, 0, len(actions))
	for _, a := range actions {
		out = append(out, actionInfo{Name: a.Name, Label: a.Label, Confirm: a.Confirm})
	}
	return out
}

type listData struct {
	pageBase
	Slug            string
	VerboseName     string
	Columns         []columnHeader
	Rows            []listRow
	Page            core.Page
	RangeStart      int
	RangeEnd        int
	Search          string
	Filters         map[string]string
	FilterControls  []filterControl
	PageSizeOptions []pageSizeOption
	PageURLs        pageURLs
	// ResetURL clears search and every filter but keeps sort and page
	// size -- those are how you're reading the table, not what you're
	// narrowing it to.
	ResetURL         string
	HasActiveFilters bool
	// ActiveFilterCount is how many declared filters are currently
	// narrowing the list -- the badge on the Filters trigger, so the
	// panel says how much it's hiding without being opened. Counted
	// here rather than in the template because html/template has no
	// arithmetic; search isn't included, since it has its own visible
	// box in the toolbar.
	ActiveFilterCount int
	Ordering          string
	ExportQuery       string
	Actions           []actionInfo
	Permissions       permissions
	Reorderable       bool
}

func (r *Renderer) buildListData(
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	page core.Page,
	req core.ListRequest,
	perms permissions,
	relationPermissions map[string]bool,
	messages []flashMessage,
) listData {
	slug := modelAdmin.Slug()
	columns := make([]columnHeader, 0, len(modelAdmin.ListDisplay()))
	for _, name := range modelAdmin.ListDisplay() {
		field, _ := modelAdmin.Field(name)
		nextSort := name
		indicator := ""
		if req.Ordering == name {
			nextSort = "-" + name
			indicator = " ▲"
		} else if req.Ordering == "-"+name {
			nextSort = name
			indicator = " ▼"
		}
		direction := ""
		if req.Ordering == name {
			direction = "asc"
		} else if req.Ordering == "-"+name {
			direction = "desc"
		}
		columns = append(columns, columnHeader{
			Label: field.Label, NextSort: nextSort, Indicator: indicator,
			Direction: direction,
			AscURL:    listURL(r.basePath, slug, req, listURLOpts{Ordering: name, HasOrder: true}),
			DescURL:   listURL(r.basePath, slug, req, listURLOpts{Ordering: "-" + name, HasOrder: true}),
		})
	}

	rows := make([]listRow, 0, len(page.Items))
	for _, obj := range page.Items {
		cells := make([]template.HTML, 0, len(modelAdmin.ListDisplay()))
		for _, name := range modelAdmin.ListDisplay() {
			field, _ := modelAdmin.Field(name)
			cells = append(cells, fieldValueHTML(r.admin, r.basePath, relationPermissions, field, field.GetValue(obj)))
		}
		rows = append(rows, listRow{PK: modelAdmin.GetPK(obj), Cells: cells})
	}

	// Django-admin-style filter sidebar: each choice is a link, not a
	// <select> option, so every choice needs its own
	// URL carrying search/ordering/other-filters and only changing the
	// one filter it represents (page resets to 1, matching search/sort
	// links elsewhere on this page).
	filterControls := make([]filterControl, 0, len(modelAdmin.Filters()))
	for _, filter := range modelAdmin.Filters() {
		current := req.Filters[filter.Name()]
		choices := make([]filterChoice, 0)
		for _, pair := range filter.ChoicesWithLabels() {
			choices = append(choices, filterChoice{
				Value:    pair[0],
				Label:    pair[1],
				Selected: pair[0] == current,
				URL:      filterChoiceURL(r.basePath, slug, req, filter.Name(), pair[0]),
			})
		}
		active := ""
		for _, choice := range choices {
			if choice.Selected && choice.Value != "" {
				active = choice.Label
			}
		}
		others := make(map[string]string, len(req.Filters))
		for name, other := range req.Filters {
			if name != filter.Name() {
				others[name] = other
			}
		}
		filterControls = append(filterControls, filterControl{
			Name: filter.Name(), Label: filter.Label(), Choices: choices,
			Active:   active,
			ClearURL: listURL(r.basePath, slug, req, listURLOpts{Filters: others, HasFilters: true}),
		})
	}

	activeFilterCount := 0
	for _, control := range filterControls {
		if control.Active != "" {
			activeFilterCount++
		}
	}

	// Rows-per-page choices. Changing the size returns to page 1 --
	// staying on page 7 while quadrupling the page size would land the
	// reader somewhere they never asked to be.
	sizeOptions := make([]pageSizeOption, 0, len(pageSizeChoices))
	for _, size := range pageSizeChoices {
		sizeOptions = append(sizeOptions, pageSizeOption{
			Size:     size,
			Selected: size == req.PageSize,
			URL:      listURL(r.basePath, slug, req, listURLOpts{PageSize: size, HasSize: true}),
		})
	}

	jumps := pageURLs{
		First: listURL(r.basePath, slug, req, listURLOpts{}),
		Last:  listURL(r.basePath, slug, req, listURLOpts{Page: page.NumPages()}),
	}
	if page.HasPrevious() {
		jumps.Previous = listURL(r.basePath, slug, req, listURLOpts{Page: page.PreviousPage()})
	}
	if page.HasNext() {
		jumps.Next = listURL(r.basePath, slug, req, listURLOpts{Page: page.NextPage()})
	}

	rangeStart, rangeEnd := 0, 0
	if page.TotalCount > 0 {
		rangeStart = (page.Number-1)*page.PageSize + 1
		rangeEnd = page.Number * page.PageSize
		if rangeEnd > page.TotalCount {
			rangeEnd = page.TotalCount
		}
	}

	return listData{
		pageBase:        r.pageBase(principal, modelAdmin.VerboseName(), modelAdmin.VerboseName(), "resource:"+slug, listBreadcrumbs(modelAdmin), messages),
		Slug:            slug,
		VerboseName:     modelAdmin.VerboseName(),
		Columns:         columns,
		Rows:            rows,
		Page:            page,
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		Search:          req.Search,
		Filters:         req.Filters,
		FilterControls:  filterControls,
		PageSizeOptions: sizeOptions,
		PageURLs:        jumps,
		ResetURL: listURL(r.basePath, slug, req, listURLOpts{
			HasSearch: true, Filters: nil, HasFilters: true,
		}),
		HasActiveFilters:  req.Search != "" || len(req.Filters) > 0,
		ActiveFilterCount: activeFilterCount,
		Ordering:          req.Ordering,
		ExportQuery:       exportQuery(req),
		Actions:           actionInfos(modelAdmin),
		Permissions:       perms,
		Reorderable:       modelAdmin.Reorderable(),
	}
}

// queryString incrementally builds a "?k=v&k=v" query string, skipping
// empty values -- shared by exportQuery and filterChoiceURL so both
// stay consistent about escaping and omitting defaults.
type queryString struct {
	values string
}

func (q *queryString) add(k, v string) {
	if v == "" {
		return
	}
	if q.values == "" {
		q.values = "?"
	} else {
		q.values += "&"
	}
	q.values += k + "=" + template.URLQueryEscaper(v)
}

func exportQuery(req core.ListRequest) string {
	var q queryString
	q.add("search", req.Search)
	for name, value := range req.Filters {
		q.add("filter["+name+"]", value)
	}
	q.add("sort", req.Ordering)
	return q.values
}

// filterChoiceURL is one link in the Django-admin-style filter sidebar
// (list.html): search, ordering, and every other filter are carried
// over unchanged; filterName is set to value (or omitted entirely when
// value is "", meaning the choice clears that filter). Page is
// deliberately omitted, resetting to page 1, matching search/sort.
func filterChoiceURL(basePath, slug string, req core.ListRequest, filterName, value string) string {
	filters := make(map[string]string, len(req.Filters))
	for name, other := range req.Filters {
		if name != filterName {
			filters[name] = other
		}
	}
	if value != "" {
		filters[filterName] = value
	}
	return listURL(basePath, slug, req, listURLOpts{Filters: filters, HasFilters: true})
}

// pageSizeChoices are the sizes the footer's rows-per-page control
// offers. defaultPageSize matches the handler's own fallback, so it's
// the one size a URL never has to spell out.
var pageSizeChoices = [...]int{10, 25, 50, 100}

const defaultPageSize = 25

// listURLOpts overrides individual parameters of the current list
// request. Go has no keyword arguments, so each override pairs with a
// Has* flag -- otherwise "clear the search" and "leave the search
// alone" would both be the zero value.
type listURLOpts struct {
	Search     string
	HasSearch  bool
	Filters    map[string]string
	HasFilters bool
	Ordering   string
	HasOrder   bool
	Page       int
	PageSize   int
	HasSize    bool
}

// listURL builds one list-view URL, carrying over every parameter from
// the current request except those explicitly overridden. Every control
// on the list page (filters, sort, paging, rows-per-page, reset) is a
// link, so they all need the same "keep what's there, change one thing"
// rule; building it once here is what keeps the templates free of
// query-string assembly. Mirrors the Python adapter's _list_url.
func listURL(basePath, slug string, req core.ListRequest, opts listURLOpts) string {
	search := req.Search
	if opts.HasSearch {
		search = opts.Search
	}
	filters := req.Filters
	if opts.HasFilters {
		filters = opts.Filters
	}
	ordering := req.Ordering
	if opts.HasOrder {
		ordering = opts.Ordering
	}
	pageSize := req.PageSize
	if opts.HasSize {
		pageSize = opts.PageSize
	}

	var q queryString
	q.add("search", search)
	// Sorted so a URL is stable across renders -- Go map iteration is
	// randomised, and an unstable href breaks both caching and tests.
	names := make([]string, 0, len(filters))
	for name := range filters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		q.add("filter["+name+"]", filters[name])
	}
	q.add("sort", ordering)
	// Page 1 and the default size are the implied state; leaving them
	// out keeps the common URL clean and makes "reset" a bare path.
	if opts.Page > 1 {
		q.add("page", strconv.Itoa(opts.Page))
	}
	if pageSize > 0 && pageSize != defaultPageSize {
		q.add("page_size", strconv.Itoa(pageSize))
	}
	return basePath + "/" + slug + q.values
}

func (r *Renderer) RenderList(principal *core.Principal, modelAdmin core.ModelAdmin, page core.Page, req core.ListRequest, perms permissions, relationPermissions map[string]bool, messages []flashMessage) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "list", r.list)
	if err != nil {
		return "", err
	}
	data := r.buildListData(principal, modelAdmin, page, req, perms, relationPermissions, messages)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r *Renderer) RenderListFragment(principal *core.Principal, modelAdmin core.ModelAdmin, page core.Page, req core.ListRequest, perms permissions, relationPermissions map[string]bool) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "list", r.list)
	if err != nil {
		return "", err
	}
	data := r.buildListData(principal, modelAdmin, page, req, perms, relationPermissions, nil)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- detail -------------------------------------------------------------

type detailField struct {
	Label string
	Value template.HTML
}

type detailData struct {
	pageBase
	Slug           string
	PK             any
	Fields         []detailField
	Actions        []actionInfo
	Permissions    permissions
	InlineSections []inlineSectionData
}

func (r *Renderer) RenderDetail(principal *core.Principal, modelAdmin core.ModelAdmin, obj any, perms permissions, relationPermissions map[string]bool, messages []flashMessage) (string, error) {
	fields := make([]detailField, 0, len(modelAdmin.DetailFields()))
	for _, name := range modelAdmin.DetailFields() {
		field, _ := modelAdmin.Field(name)
		fields = append(fields, detailField{
			Label: field.Label,
			Value: fieldValueHTML(r.admin, r.basePath, relationPermissions, field, field.GetValue(obj)),
		})
	}
	inlineSections, err := r.buildInlineSections(principal, modelAdmin, obj, "readonly", "", nil, nil, nil)
	if err != nil {
		return "", err
	}
	data := detailData{
		pageBase:       r.pageBase(principal, modelAdmin.VerboseName(), modelAdmin.VerboseName(), "resource:"+modelAdmin.Slug(), detailBreadcrumbs(modelAdmin, obj, r.basePath), messages),
		Slug:           modelAdmin.Slug(),
		PK:             modelAdmin.GetPK(obj),
		Fields:         fields,
		Actions:        actionInfos(modelAdmin),
		Permissions:    perms,
		InlineSections: inlineSections,
	}
	tmpl, err := r.contentTemplate(modelAdmin, "detail", r.detail)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- form -------------------------------------------------------------

type formData struct {
	pageBase
	VerboseName string
	FormAction  string
	// Slug and PK are set only when editing an existing record, so
	// form.html can link its Delete button; on create they stay zero and
	// the button isn't rendered at all.
	Slug           string
	PK             any
	Permissions    permissions
	NonFieldErrors []string
	Inputs         []template.HTML
	InlineSections []inlineSectionData
}

func (r *Renderer) RenderForm(
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "form", r.form)
	if err != nil {
		return "", err
	}
	return r.executeForm(tmpl, "base", principal, modelAdmin, obj, submitted, errs, relationOptions)
}

func (r *Renderer) RenderFormFragment(
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "form", r.form)
	if err != nil {
		return "", err
	}
	return r.executeForm(tmpl, "content", principal, modelAdmin, obj, submitted, errs, relationOptions)
}

func (r *Renderer) executeForm(
	tmpl *template.Template,
	name string,
	principal *core.Principal,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
) (string, error) {
	verb, action := "Create", fmt.Sprintf("%s/%s/create", r.basePath, modelAdmin.Slug())
	if obj != nil {
		verb = "Edit"
		action = fmt.Sprintf("%s/%s/%v/edit", r.basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj))
	}

	inputs := make([]template.HTML, 0, len(modelAdmin.FormFields()))
	for _, fieldName := range modelAdmin.FormFields() {
		field, _ := modelAdmin.Field(fieldName)
		value, ok := submitted[fieldName]
		if !ok && obj != nil {
			value = field.GetValue(obj)
		}
		input, err := r.formInputHTML(r.basePath, field, value, errs[fieldName], relationOptions[fieldName])
		if err != nil {
			return "", err
		}
		inputs = append(inputs, input)
	}

	mode := "placeholder"
	if obj != nil {
		mode = "edit"
	}
	inlineSections, err := r.buildInlineSections(principal, modelAdmin, obj, mode, "", nil, nil, nil)
	if err != nil {
		return "", err
	}

	data := formData{
		pageBase:    r.pageBase(principal, fmt.Sprintf("%s %s", verb, modelAdmin.VerboseName()), fmt.Sprintf("%s %s", verb, modelAdmin.VerboseName()), "resource:"+modelAdmin.Slug(), formBreadcrumbs(modelAdmin, obj, r.basePath), nil),
		VerboseName: modelAdmin.VerboseName(),
		FormAction:  action,
		// The edit form offers Delete in its action bar, so it needs the
		// same permission map the detail page gets -- otherwise the
		// button would render for a principal the authorizer would then
		// reject at the route.
		Permissions:    computePermissions(r.admin, principal, modelAdmin),
		Inputs:         inputs,
		InlineSections: inlineSections,
	}
	if obj != nil {
		data.Slug = modelAdmin.Slug()
		data.PK = modelAdmin.GetPK(obj)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- inlines (Django-admin StackedInline/TabularInline style) -----------
//
// See core/inline.go and docs/inlines.md. inlineSectionData/
// inlineRowData/inlineAddRowData carry pre-rendered template.HTML
// cells (via formInputHTML/inlineTableCellHTML for editable rows,
// fieldValueHTML/inlineDetailRowHTML for read-only ones) -- same
// "pre-render in Go, template just prints" convention as
// formData.Inputs/detailData.Fields. Which cell-building function
// runs is decided here in Go by inline.Layout, not deferred to the
// template, so admin/inline.html's stacked/tabular defines can stay a
// uniform `{{range .Cells}}{{.}}{{end}}` either way.

type inlineColumn struct {
	Name  string
	Label string
}

type inlineRowData struct {
	PK        any
	UpdateURL string
	DeleteURL string
	DetailURL string
	Cells     []template.HTML
}

type inlineAddRowData struct {
	CreateURL string
	Cells     []template.HTML
}

type inlineSectionData struct {
	Slug      string
	Label     string
	Layout    string // core.InlineLayoutStacked | core.InlineLayoutTabular
	Mode      string // "placeholder" | "edit" | "readonly"
	CanChange bool
	CanDelete bool
	// Columns holds header labels for the tabular layout only (column
	// i corresponds to Rows[*].Cells[i] and AddRow.Cells[i]); the
	// stacked layout doesn't need it since each cell already carries
	// its own label (formInputHTML) or is wrapped with one
	// (inlineDetailRowHTML).
	Columns []inlineColumn
	Rows    []inlineRowData
	AddRow  *inlineAddRowData // nil unless Mode == "edit" && the principal has the child's own create permission
}

// buildInlineSections builds one inlineSectionData per Inline the
// principal may view -- reused by RenderForm/RenderFormFragment (via
// executeForm), RenderDetail, and RenderInlineFragment, so all three
// call sites can never drift out of sync (the same "full page and
// fragment share the same context builder" principle this package's
// own doc comment already states for list/form fragments).
//
// redisplayChildSlug/redisplayPK/redisplayData/redisplayErrs carry a
// failed inline mutation's submitted values back into just the one
// row (or the add-row, if redisplayPK is nil) being redisplayed --
// zero values everywhere else, since only RenderInlineFragment ever
// has a redisplay to show.
func (r *Renderer) buildInlineSections(
	principal *core.Principal, modelAdmin core.ModelAdmin, obj any, mode string,
	redisplayChildSlug string, redisplayPK any, redisplayData map[string]any, redisplayErrs map[string][]string,
) ([]inlineSectionData, error) {
	var out []inlineSectionData
	for _, inline := range modelAdmin.Inlines() {
		childAdmin, ok := r.admin.GetModelAdmin(inline.Child)
		if !ok || !childAdmin.CanView() {
			continue
		}
		if r.admin.Authorizer != nil && !r.admin.Authorizer.Can(principal, core.ResourcePermission(inline.Child, "view"), childAdmin) {
			continue
		}
		childPerms := computePermissions(r.admin, principal, childAdmin)

		section := inlineSectionData{
			Slug: inline.Child, Label: inlineLabel(r.admin, inline), Layout: inline.Layout, Mode: mode,
			CanChange: childPerms.CanUpdate, CanDelete: childPerms.CanDelete,
		}
		if mode == "placeholder" {
			out = append(out, section)
			continue
		}

		fieldNames := excluding(childAdmin.FormFields(), inline.FKField)
		detailNames := excluding(childAdmin.DetailFields(), inline.FKField)
		columnNames := fieldNames
		if mode == "readonly" {
			columnNames = detailNames
		}
		if inline.Layout == core.InlineLayoutTabular {
			for _, name := range columnNames {
				field, _ := childAdmin.Field(name)
				section.Columns = append(section.Columns, inlineColumn{Name: name, Label: field.Label})
			}
		}
		relPerms := computeRelationPermissions(r.admin, principal, childAdmin, detailNames)

		parentPK := modelAdmin.GetPK(obj)
		children, err := core.FilterInlineChildren(context.Background(), childAdmin, inline.FKField, modelAdmin, parentPK)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			childPK := childAdmin.GetPK(child)
			var errs map[string][]string
			var data map[string]any
			if inline.Child == redisplayChildSlug && redisplayPK != nil && fmt.Sprint(childPK) == fmt.Sprint(redisplayPK) {
				errs, data = redisplayErrs, redisplayData
			}
			var cells []template.HTML
			if mode == "edit" {
				relOptions := computeRelationOptions(r.admin, childAdmin, child)
				for _, name := range fieldNames {
					field, _ := childAdmin.Field(name)
					value := field.GetValue(child)
					if data != nil {
						value = data[name]
					}
					if inline.Layout == core.InlineLayoutTabular {
						cells = append(cells, inlineTableCellHTML(r.basePath, field, value, errs[name], relOptions[name]))
					} else {
						input, err := r.formInputHTML(r.basePath, field, value, errs[name], relOptions[name])
						if err != nil {
							return nil, err
						}
						cells = append(cells, input)
					}
				}
			} else {
				for _, name := range detailNames {
					field, _ := childAdmin.Field(name)
					valueHTML := fieldValueHTML(r.admin, r.basePath, relPerms, field, field.GetValue(child))
					if inline.Layout == core.InlineLayoutTabular {
						cells = append(cells, valueHTML)
					} else {
						cells = append(cells, inlineDetailRowHTML(field.Label, valueHTML))
					}
				}
			}
			section.Rows = append(section.Rows, inlineRowData{
				PK:        childPK,
				Cells:     cells,
				UpdateURL: fmt.Sprintf("%s/%s/%v/inlines/%s/%v", r.basePath, modelAdmin.Slug(), parentPK, inline.Child, childPK),
				DeleteURL: fmt.Sprintf("%s/%s/%v/inlines/%s/%v", r.basePath, modelAdmin.Slug(), parentPK, inline.Child, childPK),
				DetailURL: fmt.Sprintf("%s/%s/%v", r.basePath, inline.Child, childPK),
			})
		}

		if mode == "edit" && childPerms.CanCreate {
			var errs map[string][]string
			var data map[string]any
			if inline.Child == redisplayChildSlug && redisplayPK == nil {
				errs, data = redisplayErrs, redisplayData
			}
			relOptions := computeRelationOptions(r.admin, childAdmin, nil)
			var cells []template.HTML
			for _, name := range fieldNames {
				field, _ := childAdmin.Field(name)
				var value any
				if data != nil {
					value = data[name]
				}
				if inline.Layout == core.InlineLayoutTabular {
					cells = append(cells, inlineTableCellHTML(r.basePath, field, value, errs[name], relOptions[name]))
				} else {
					input, err := r.formInputHTML(r.basePath, field, value, errs[name], relOptions[name])
					if err != nil {
						return nil, err
					}
					cells = append(cells, input)
				}
			}
			section.AddRow = &inlineAddRowData{
				CreateURL: fmt.Sprintf("%s/%s/%v/inlines/%s", r.basePath, modelAdmin.Slug(), parentPK, inline.Child),
				Cells:     cells,
			}
		}

		out = append(out, section)
	}
	return out, nil
}

// RenderInlineFragment renders just the one inline section matching
// inline.Child, standalone -- the response body for the three inline
// create/update/delete routes (see fiber/handlers.go's
// handleInlineCreate/Update/Delete), swapped into
// #inline-{child_slug} via HTMX's outerHTML on every mutation.
func (r *Renderer) RenderInlineFragment(
	principal *core.Principal, modelAdmin core.ModelAdmin, obj any, inline core.Inline,
	redisplayPK any, redisplayData map[string]any, redisplayErrs map[string][]string,
) (string, error) {
	sections, err := r.buildInlineSections(principal, modelAdmin, obj, "edit", inline.Child, redisplayPK, redisplayData, redisplayErrs)
	if err != nil {
		return "", err
	}
	var section inlineSectionData
	for _, s := range sections {
		if s.Slug == inline.Child {
			section = s
			break
		}
	}
	var buf bytes.Buffer
	if err := r.inlineFragment.ExecuteTemplate(&buf, "content", section); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- delete -------------------------------------------------------------

type deleteData struct {
	pageBase
	VerboseName string
}

func (r *Renderer) RenderDelete(principal *core.Principal, modelAdmin core.ModelAdmin, obj any) (string, error) {
	data := deleteData{
		pageBase:    r.pageBase(principal, "Delete "+modelAdmin.VerboseName(), "Delete "+modelAdmin.VerboseName(), "resource:"+modelAdmin.Slug(), deleteBreadcrumbs(modelAdmin, obj, r.basePath), nil),
		VerboseName: modelAdmin.VerboseName(),
	}
	tmpl, err := r.contentTemplate(modelAdmin, "delete", r.deleteTpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- dashboard -------------------------------------------------------------

type renderedWidget struct {
	Title string
	Size  string
	Icon  string
	Body  template.HTML
}

type dashboardData struct {
	pageBase
	Widgets []renderedWidget
}

// widgetIcons maps a widget's Template() (see core/widget.go) to the
// icon shown in its dashboard card badge, mirroring the Python side's
// admin/dashboard.html widget_icons lookup.
var widgetIcons = map[string]string{
	"admin/widgets/metric.html":   "metric",
	"admin/widgets/progress.html": "progress",
	"admin/widgets/table.html":    "table",
	"admin/widgets/chart.html":    "chart",
	"admin/widgets/activity.html": "activity",
	"admin/widgets/donut.html":    "donut",
	"admin/widgets/stat.html":     "stat",
	"admin/widgets/timeline.html": "timeline",
	"admin/widgets/tabs.html":     "tabs",
}

// renderedPanel is one already-rendered child of a core.Container
// widget, as admin/widgets/tabs.html consumes it.
type renderedPanel struct {
	Label string
	Body  template.HTML
}

// maxWidgetDepth bounds how deeply container widgets may nest, so a
// Tabs panel that (directly or transitively) contains itself fails
// with a clear error instead of recursing until the stack runs out.
const maxWidgetDepth = 8

// widgetTemplate returns the *template.Template that defines
// widget.Template()'s name: the framework's own pre-built set if it's
// one of the built-in widgets, otherwise (application-supplied
// WithTemplateDirs only) a freshly-resolved, cached one -- the same
// "pay only if you use it" shape as contentTemplate, closing the
// custom-widget-template gap that per-resource overrides alone don't
// cover (widgets aren't tied to any one ModelAdmin). A custom widget's
// template file must define a block named after its own Template()
// value, the same convention the built-in widget templates use.
func (r *Renderer) widgetTemplate(name string) (*template.Template, error) {
	if r.widgets.Lookup(name) != nil {
		return r.widgets, nil
	}
	cacheKey := "widget|" + name
	r.overrideMu.RLock()
	cached, ok := r.overrideCache[cacheKey]
	r.overrideMu.RUnlock()
	if ok {
		return cached, nil
	}
	for _, dir := range r.templateDirs {
		if _, statErr := os.Stat(dir + "/" + name); statErr != nil {
			continue
		}
		tmpl, err := template.New(path.Base(name)).Funcs(templateFuncs).ParseFS(coretemplates.FS, uiComponentsGlob)
		if err != nil {
			return nil, err
		}
		if tmpl, err = tmpl.ParseFS(os.DirFS(dir), name); err != nil {
			return nil, err
		}
		r.overrideMu.Lock()
		r.overrideCache[cacheKey] = tmpl
		r.overrideMu.Unlock()
		return tmpl, nil
	}
	return nil, fmt.Errorf("polyadmin: no widget template named %q", name)
}

// renderWidgetBody executes one widget's template against its own
// GetData. A widget implementing core.Container has its panels
// rendered first and receives them as HTML instead: html/template
// can't execute a template whose name is only known at runtime, so
// the recursion has to happen out here rather than inside
// admin/widgets/tabs.html.
func (r *Renderer) renderWidgetBody(widget core.Widget, depth int) (template.HTML, error) {
	if depth > maxWidgetDepth {
		return "", fmt.Errorf("polyadmin: widget %q nests more than %d levels deep (a container widget's panels contain itself?)", widget.Template(), maxWidgetDepth)
	}
	tmpl, err := r.widgetTemplate(widget.Template())
	if err != nil {
		return "", err
	}
	data := widget.GetData()
	if container, ok := widget.(core.Container); ok {
		panels := make([]renderedPanel, 0, len(container.Panels()))
		for _, panel := range container.Panels() {
			body, err := r.renderWidgetBody(panel.Widget, depth+1)
			if err != nil {
				return "", err
			}
			panels = append(panels, renderedPanel{Label: panel.Label, Body: body})
		}
		data = map[string]any{"Panels": panels}
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, widget.Template(), data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func (r *Renderer) RenderDashboard(principal *core.Principal, dashboard core.Dashboard, widgets []core.Widget) (string, error) {
	rendered := make([]renderedWidget, 0, len(widgets))
	for _, widget := range widgets {
		body, err := r.renderWidgetBody(widget, 0)
		if err != nil {
			return "", err
		}
		icon := widgetIcons[widget.Template()]
		if icon == "" {
			icon = "metric"
		}
		rendered = append(rendered, renderedWidget{Title: widget.Title(), Size: widget.Size(), Icon: icon, Body: body})
	}
	title := dashboard.Title
	if title == "" {
		title = "Dashboard"
	}
	// A single active crumb -- since base.html has no separate <h1>,
	// this is the only page-title element the dashboard gets.
	data := dashboardData{pageBase: r.pageBase(principal, title, title, "", []breadcrumb{{Label: title, Active: true}}, nil), Widgets: rendered}
	var buf bytes.Buffer
	if err := r.dashboard.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- pages ----------------------------------------------------------------

type pageData struct {
	pageBase
	Page core.AdminPage
	Data any // handler-supplied extra template data
}

// PageTemplate resolves and caches templateName for a custom AdminPage
// the same way contentTemplate resolves a ModelAdmin override:
// base+toasts+modal parsed from the framework's embedded FS, then
// templateName parsed from the first WithTemplateDirs directory that
// has it. Unlike ModelAdmin views, there's no framework-default
// fallback -- a page template is always application-supplied.
func (r *Renderer) PageTemplate(templateName string) (*template.Template, error) {
	cacheKey := "page|" + templateName
	r.overrideMu.RLock()
	cached, ok := r.overrideCache[cacheKey]
	r.overrideMu.RUnlock()
	if ok {
		return cached, nil
	}
	for _, dir := range r.templateDirs {
		if _, statErr := os.Stat(dir + "/" + templateName); statErr != nil {
			continue
		}
		tmpl, err := template.New(path.Base(templateName)).Funcs(templateFuncs).
			ParseFS(coretemplates.FS, layoutFiles...)
		if err != nil {
			return nil, err
		}
		if tmpl, err = tmpl.ParseFS(coretemplates.FS, uiComponentsGlob); err != nil {
			return nil, err
		}
		if tmpl, err = tmpl.ParseFS(os.DirFS(dir), templateName); err != nil {
			return nil, err
		}
		r.overrideMu.Lock()
		r.overrideCache[cacheKey] = tmpl
		r.overrideMu.Unlock()
		return tmpl, nil
	}
	return nil, fmt.Errorf("polyadmin: no page template named %q found in template dirs %v", templateName, r.templateDirs)
}

// RenderPage renders a custom AdminPage's own template inside the
// shared admin layout (sidebar, breadcrumbs, flash toasts). data is
// handed to the template as .Data, alongside .Page (the AdminPage
// itself, for label/path access).
func (r *Renderer) RenderPage(principal *core.Principal, page core.AdminPage, templateName string, data any, messages []flashMessage) (string, error) {
	tmpl, err := r.PageTemplate(templateName)
	if err != nil {
		return "", err
	}
	breadcrumbs := append(categoryBreadcrumb(page.Category), breadcrumb{Label: page.Label, Active: true})
	pd := pageData{
		pageBase: r.pageBase(principal, page.Label, page.Label, "page:"+page.Path, breadcrumbs, messages),
		Page:     page,
		Data:     data,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pd); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// -- lookup -------------------------------------------------------------

type lookupOption struct {
	PK    any
	Label string
}

func (r *Renderer) RenderLookup(options []lookupOption) (string, error) {
	var buf bytes.Buffer
	if err := r.lookup.ExecuteTemplate(&buf, "lookup", options); err != nil {
		return "", err
	}
	return buf.String(), nil
}
