// Package fiber mounts a *core.Admin as routes on a *fiber.App. Field and
// form HTML is built in Go (render_helpers.go) rather than in the
// html/template files, for tighter control over escaping.
package fiber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MagicRodri/go-polyadmin/core"
	coretemplates "github.com/MagicRodri/go-polyadmin/templates"

	"github.com/gofiber/fiber/v2"
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

// navGroupIcon is fixed, not per-category: a category is a string, not an
// object with settings of its own.
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

// breadcrumb is one entry in a page's trail. An empty URL means not a link
// (a category segment has no route; the current page needs none), and
// Active tells those two apart. base.html prepends the home crumb itself,
// so these lists never include it.
type breadcrumb struct {
	Label  string
	URL    string
	Active bool
}

// categoryBreadcrumb is the category crumb, if any: the first segment
// after the home crumb, never a link and never active.
func (r *Renderer) categoryBreadcrumb(category string) []breadcrumb {
	if category == "" {
		return nil
	}
	return []breadcrumb{{Label: r.t(category)}}
}

type pageBase struct {
	// Principal is who is signed in, for the sidebar footer's NavUser
	// (shadcn sidebar-07). Every page renders a sidebar, so it has to
	// reach every page's data.
	Principal *core.Principal
	// CSRFToken is per-request state like Principal, and reaches every
	// page for the same reason: base.html renders it as a meta tag, and
	// every no-JS form renders it as a hidden field.
	CSRFToken string
	Title     string
	Header    string
	BasePath  string
	// CurrentSlug is the bare resource slug, used by form.html/delete.html to
	// link back to the list view. CurrentNavKey is the sidebar highlight key
	// ("resource:"+slug, "page:"+path, or "").
	CurrentSlug   string
	CurrentNavKey string
	// ListToken is the list this page was reached from -- preserve_filters
	// (docs/lists.md). Forms post it back in a hidden field; "" when there
	// is none.
	ListToken string
	NavItems  []navEntry
	// SiteTitle is translated; SiteInitials, the avatar fallback, comes
	// from the untranslated title, so it stays the site's own mark.
	SiteTitle    string
	SiteInitials string
	SiteLogoURL  string
	Breadcrumbs  []breadcrumb
	Messages     []flashMessage
	// CanSignOut is whether a core.LoginBackend is configured -- i.e.
	// whether there is a session to end. Without one the admin has no
	// logout route, so offering the control would be a dead button.
	CanSignOut bool
}

// buildNav returns sidebar entries, flat links and category groups
// interleaved in first-registration order. Entries the principal cannot
// view, and pages with HideFromNav, are omitted. A group's Expanded flag
// seeds the accordion's initial Alpine `open` state and is true iff it
// holds the current page.
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

func (r *Renderer) pageBase(principal *core.Principal, csrfToken, title, header, navKey string, breadcrumbs []breadcrumb, messages []flashMessage) pageBase {
	siteTitle := r.admin.SiteTitle
	if siteTitle == "" {
		siteTitle = "PolyAdmin"
	}
	var currentSlug string
	if slug, ok := strings.CutPrefix(navKey, "resource:"); ok {
		currentSlug = slug
	}
	return pageBase{
		Principal: principal, CSRFToken: csrfToken,
		Title: title, Header: header, BasePath: r.basePath,
		CurrentSlug: currentSlug, CurrentNavKey: navKey, NavItems: r.buildNav(navKey),
		SiteTitle: r.t(siteTitle), SiteInitials: siteInitials(siteTitle), SiteLogoURL: r.admin.SiteLogoURL,
		Breadcrumbs: breadcrumbs, Messages: messages,
		CanSignOut: r.admin.LoginBackend != nil,
	}
}

// objectLabel names an object in a breadcrumb trail: the first
// SearchFields entry (usually the most identifying), else the first
// ListDisplay column, else the primary key.
// prepopulatedJSON describes prepopulated_fields for the client: which
// field is filled from which, and whether its slug keeps its own letters.
// Empty on an edit form -- an existing record's slug is a real identifier,
// and rewriting it from the title is how links rot.
func prepopulatedJSON(modelAdmin core.ModelAdmin, obj any) string {
	fields := modelAdmin.Prepopulated()
	if obj != nil || len(fields) == 0 {
		return ""
	}
	unicode := make(map[string]bool)
	if base, ok := modelAdmin.(interface{ UnicodeSlugFields() []string }); ok {
		for _, name := range base.UnicodeSlugFields() {
			unicode[name] = true
		}
	}
	type spec struct {
		From    []string `json:"from"`
		Unicode bool     `json:"unicode,omitempty"`
	}
	out := make(map[string]spec, len(fields))
	for target, sources := range fields {
		out[target] = spec{From: sources, Unicode: unicode[target]}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// pkColumn is the column that opens the record from a readonly inline
// row: the primary key's own when it is shown -- an id is the one cell
// that is never a link already and never wraps -- and the first column
// otherwise, so a row is always reachable.
//
// GetPK reads a value, not a field name (a ModelAdmin may override it),
// so the column is found by name.
func pkColumn(names []string) string {
	if len(names) == 0 {
		return ""
	}
	for _, name := range names {
		if strings.EqualFold(name, "id") {
			return name
		}
	}
	return names[0]
}

// linkedCell wraps a rendered list cell in a link to its record, which is
// what list_display_links asks for. A cell that already contains an anchor
// -- a relation, chiefly -- is left alone: nesting <a> inside <a> is
// invalid HTML that browsers silently restructure.
func linkedCell(cell template.HTML, url string) template.HTML {
	if strings.Contains(string(cell), "<a ") {
		return cell
	}
	classes, err := uiClasses("text", "link")
	if err != nil {
		return cell
	}
	return template.HTML(fmt.Sprintf(`<a href="%s" class="%s">%s</a>`, template.HTMLEscapeString(url), classes, cell))
}

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

// The breadcrumb builders translate the resource's name and their own
// literals; the object's label is data and stays as it is.
func (r *Renderer) listBreadcrumbs(modelAdmin core.ModelAdmin) []breadcrumb {
	return append(r.categoryBreadcrumb(modelAdmin.Category()), breadcrumb{Label: r.t(modelAdmin.VerboseName()), Active: true})
}

func (r *Renderer) detailBreadcrumbs(modelAdmin core.ModelAdmin, obj any, listToken string) []breadcrumb {
	crumbs := r.categoryBreadcrumb(modelAdmin.Category())
	return append(crumbs,
		breadcrumb{Label: r.t(modelAdmin.VerboseName()), URL: r.listCrumbURL(modelAdmin, listToken)},
		breadcrumb{Label: objectLabel(modelAdmin, obj), Active: true},
	)
}

func (r *Renderer) formBreadcrumbs(modelAdmin core.ModelAdmin, obj any, listToken string) []breadcrumb {
	crumbs := r.categoryBreadcrumb(modelAdmin.Category())
	crumbs = append(crumbs, breadcrumb{Label: r.t(modelAdmin.VerboseName()), URL: r.listCrumbURL(modelAdmin, listToken)})
	if obj != nil {
		crumbs = append(crumbs,
			breadcrumb{Label: objectLabel(modelAdmin, obj), URL: core.WithListToken(fmt.Sprintf("%s/%s/%v", r.basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj)), listToken)},
			breadcrumb{Label: r.t("Edit"), Active: true},
		)
	} else {
		crumbs = append(crumbs, breadcrumb{Label: r.t("New"), Active: true})
	}
	return crumbs
}

func (r *Renderer) deleteBreadcrumbs(modelAdmin core.ModelAdmin, obj any, listToken string) []breadcrumb {
	crumbs := r.categoryBreadcrumb(modelAdmin.Category())
	return append(crumbs,
		breadcrumb{Label: r.t(modelAdmin.VerboseName()), URL: r.listCrumbURL(modelAdmin, listToken)},
		breadcrumb{Label: objectLabel(modelAdmin, obj), URL: core.WithListToken(fmt.Sprintf("%s/%s/%v", r.basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj)), listToken)},
		breadcrumb{Label: r.t("Delete"), Active: true},
	)
}

// listCrumbURL is the breadcrumb back to the list: the one the page was
// reached from when preserve_filters handed us a token, the bare list
// otherwise. This is where a user actually returns, so it is the crumb
// that matters most for preserving filters.
func (r *Renderer) listCrumbURL(modelAdmin core.ModelAdmin, listToken string) string {
	if listToken != "" {
		return listToken
	}
	return fmt.Sprintf("%s/%s", r.basePath, modelAdmin.Slug())
}

type Renderer struct {
	admin    *core.Admin
	basePath string
	// locale is the one locale this Renderer's template sets are bound
	// to; funcs are those sets' template functions (see localeFuncs).
	locale string
	funcs  template.FuncMap
	// errorPage is the error template for this locale -- see errors.go.
	errorPage *template.Template

	list              *template.Template
	detail            *template.Template
	form              *template.Template
	deleteTpl         *template.Template
	deleteSelectedTpl *template.Template
	dashboard         *template.Template
	// login renders without layoutFiles: it is the one full page that
	// is not framed by the admin shell -- see admin/login.html.
	login          *template.Template
	widgets        *template.Template
	lookup         *template.Template
	inlineFragment *template.Template

	// uiSet holds the component partials alone, so Go code building HTML
	// directly (formInputHTML's Calendar popover) can render one instead of
	// duplicating its markup as a Go string.
	uiSet *template.Template

	// templateDirs are application override directories, searched in order
	// before the embedded templates. See WithTemplateDirs.
	templateDirs []string

	// overrideCache holds template sets contentTemplate resolves lazily. A
	// built *template.Template is safe for concurrent execution, but building
	// and caching one needs the lock, since handlers run concurrently.
	overrideMu    sync.RWMutex
	overrideCache map[string]*template.Template
}

// layoutFiles are the templates every full-page set needs. One list, so
// adding a shared partial doesn't mean editing six ParseFS calls and
// forgetting the override path.
var layoutFiles = []string{
	"admin/base.html",
	"admin/theme.html",
	"admin/components/toasts.html",
	"admin/components/action_confirm_modal.html",
}

// uiComponentsGlob matches the component partials, parsed into every
// template set. Unlike the `ui` func, which yields a class string, these
// are whole markup+Alpine blocks: {{template "ui/dialog" dict ...}}.
const uiComponentsGlob = "admin/components/ui/*.html"

// sharedPartials are non-component framework partials any set may invoke.
// They go into every set, fragments included: Go resolves {{template}}
// names at execution time, so a set missing a dependency fails only when
// that page is rendered.
var sharedPartials = []string{
	"admin/components/csrf-field.html",
}

// listPartials is the list view's file set. The page template is a shim
// over components/list_content.html, the swappable #resource-list region
// the fragment route renders on its own.
var listPartials = []string{
	"admin/components/search.html",
	"admin/components/list_content.html",
	"admin/resource/list.html",
}

// parseComponents parses the shadcn ui partials plus sharedPartials
// into tmpl -- the pair every template set needs, whether or not it
// also gets layoutFiles.
func parseComponents(tmpl *template.Template) (*template.Template, error) {
	tmpl, err := tmpl.ParseFS(coretemplates.FS, uiComponentsGlob)
	if err != nil {
		return nil, err
	}
	return tmpl.ParseFS(coretemplates.FS, sharedPartials...)
}

// buildTemplate parses the shared layout + ui component partials plus
// the given content files into one set.
func buildTemplate(funcs template.FuncMap, contentFiles ...string) (*template.Template, error) {
	files := append(append([]string{}, layoutFiles...), contentFiles...)
	tmpl, err := template.New(path.Base(files[0])).Funcs(funcs).ParseFS(coretemplates.FS, files...)
	if err != nil {
		return nil, err
	}
	return parseComponents(tmpl)
}

// NewRenderer builds the Renderer for the admin's default locale. Mount
// uses NewRenderers; this remains for callers that need one set.
func NewRenderer(admin *core.Admin, basePath string, templateDirs ...string) (*Renderer, error) {
	i18n, err := core.NewI18n(admin)
	if err != nil {
		return nil, err
	}
	return newRenderer(admin, i18n, i18n.Default, basePath, templateDirs...)
}

func newRenderer(admin *core.Admin, i18n *core.I18n, locale, basePath string, templateDirs ...string) (*Renderer, error) {
	r := &Renderer{
		admin: admin, basePath: basePath, locale: locale,
		funcs:        localeFuncs(i18n.Translator, locale, switcherFor(admin, i18n, locale)),
		templateDirs: templateDirs, overrideCache: make(map[string]*template.Template),
	}
	// Fragment-only sets (widgets, lookup, inline) render without the
	// base layout, so they take the ui partials and funcs but not
	// layoutFiles.
	buildFragment := func(files ...string) (*template.Template, error) {
		tmpl, err := template.New(path.Base(files[0])).Funcs(r.funcs).ParseFS(coretemplates.FS, files...)
		if err != nil {
			return nil, err
		}
		return parseComponents(tmpl)
	}
	var err error
	if r.list, err = buildTemplate(r.funcs, listPartials...); err != nil {
		return nil, err
	}
	if r.detail, err = buildTemplate(r.funcs, "admin/components/inline.html", "admin/resource/detail.html"); err != nil {
		return nil, err
	}
	if r.form, err = buildTemplate(r.funcs, "admin/components/inline.html", "admin/components/form_wrapper.html", "admin/resource/form.html"); err != nil {
		return nil, err
	}
	if r.deleteTpl, err = buildTemplate(r.funcs, "admin/resource/delete.html"); err != nil {
		return nil, err
	}
	if r.deleteSelectedTpl, err = buildTemplate(r.funcs, "admin/resource/delete_selected.html"); err != nil {
		return nil, err
	}
	if r.dashboard, err = buildTemplate(r.funcs, "admin/dashboard.html"); err != nil {
		return nil, err
	}
	// Not a fragment, but not a base.html page either: it needs the
	// theme block (tokens, dark mode) and the ui partials, and nothing
	// else the layout would bring.
	if r.login, err = buildFragment("admin/theme.html", "admin/login.html"); err != nil {
		return nil, err
	}
	if r.widgets, err = buildFragment("admin/widgets/*.html"); err != nil {
		return nil, err
	}
	if r.lookup, err = buildFragment("admin/components/lookup_results.html"); err != nil {
		return nil, err
	}
	if r.inlineFragment, err = buildFragment("admin/components/inline.html", "admin/components/inline_fragment.html"); err != nil {
		return nil, err
	}
	if r.errorPage, err = buildFragment("admin/theme.html", "admin/error.html", "admin/components/error_fragment.html"); err != nil {
		return nil, err
	}
	if r.uiSet, err = parseComponents(template.New("ui").Funcs(r.funcs)); err != nil {
		return nil, err
	}
	return r, nil
}

// switcherFor is the language switcher's data for a Renderer's pages, or
// nil when the switcher is off: disabled by the host, or one locale only.
// Rendered by ui/locale-switcher via the {{localeSwitcher}} func; computed
// here because it is bound into funcs at parse time (see localeFuncs).
func switcherFor(admin *core.Admin, i18n *core.I18n, locale string) *localeSwitcher {
	if admin.DisableLocaleSwitcher || len(i18n.Supported) < 2 {
		return nil
	}
	return &localeSwitcher{Options: i18n.Options(), Current: locale}
}

// t translates a string built in Go for this Renderer's locale.
func (r *Renderer) t(msgid string, args ...any) string {
	return r.funcs["t"].(func(string, ...any) string)(msgid, args...)
}

// tn is t's plural form.
func (r *Renderer) tn(singular, plural string, n int, args ...any) string {
	return r.funcs["tn"].(func(string, string, any, ...any) string)(singular, plural, n, args...)
}

// Renderers holds one Renderer per supported locale: html/template binds
// functions at parse time, so a per-request locale means per-locale sets.
type Renderers struct {
	byLocale map[string]*Renderer
	fallback *Renderer
}

func NewRenderers(admin *core.Admin, i18n *core.I18n, basePath string, templateDirs ...string) (*Renderers, error) {
	rs := &Renderers{byLocale: make(map[string]*Renderer, len(i18n.Supported))}
	for _, locale := range i18n.Supported {
		r, err := newRenderer(admin, i18n, locale, basePath, templateDirs...)
		if err != nil {
			return nil, err
		}
		rs.byLocale[locale] = r
	}
	rs.fallback = rs.byLocale[i18n.Default]
	return rs, nil
}

func (rs *Renderers) byLocaleOrDefault(locale string) *Renderer {
	if r, ok := rs.byLocale[locale]; ok {
		return r
	}
	return rs.fallback
}

// For is the Renderer for the request's resolved locale.
func (rs *Renderers) For(c *fiber.Ctx) *Renderer {
	return rs.byLocaleOrDefault(core.Locale(c.Context()))
}

// uiHTML renders one component partial for the Go-built markup in
// render_helpers.go. It returns template.HTML because the caller assembles
// a larger string by hand and the partial's output is already escaped.
func (r *Renderer) uiHTML(name string, data any) (template.HTML, error) {
	var buf bytes.Buffer
	if err := r.uiSet.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// frameworkViewTemplate is the built-in template for a resource view. It
// sits under admin/resource/, the same namespace a resource's own
// override lives in, so the two are neighbours.
func frameworkViewTemplate(view string) string {
	return "admin/resource/" + view + ".html"
}

// templateCandidates returns, in priority order: an explicit override, a
// resource-specific template, then the framework default.
func templateCandidates(modelAdmin core.ModelAdmin, view string) []string {
	candidates := make([]string, 0, 3)
	if override := modelAdmin.TemplateOverride(view); override != "" {
		candidates = append(candidates, override)
	}
	candidates = append(candidates,
		"admin/resource/"+modelAdmin.Slug()+"/"+view+".html",
		frameworkViewTemplate(view),
	)
	return candidates
}

// resolveContentSource returns the first candidate that exists, checking
// the application's template dirs before the embedded ones.
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

// contentTemplate returns the template to render `view`: the eagerly-built
// `fallback`, unless an override or resource-specific file resolves to
// something else, in which case a fresh set is built from whichever
// filesystem won and cached. An application that configures neither hits
// the fallback on the first check and never reaches the cache.
func (r *Renderer) contentTemplate(modelAdmin core.ModelAdmin, view string, fallback *template.Template) (*template.Template, error) {
	if len(r.templateDirs) == 0 && modelAdmin.TemplateOverride(view) == "" {
		return fallback, nil
	}
	candidates := templateCandidates(modelAdmin, view)
	name, source, err := r.resolveContentSource(candidates)
	if err != nil {
		return nil, err
	}
	if source == fs.FS(coretemplates.FS) && name == frameworkViewTemplate(view) {
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
		// {{template "inlineSection" .}} -- see components/inline.html.
		baseFiles = append(baseFiles, "admin/components/inline.html")
	}
	// The partials the page templates are shims over, so an override keeping
	// most of a page can invoke them by name instead of copying their markup.
	switch view {
	case "list":
		baseFiles = append(baseFiles, "admin/components/search.html", "admin/components/list_content.html")
	case "form":
		baseFiles = append(baseFiles, "admin/components/form_wrapper.html")
	}
	tmpl, err := template.New(path.Base(name)).Funcs(r.funcs).ParseFS(coretemplates.FS, baseFiles...)
	if err != nil {
		return nil, err
	}
	if tmpl, err = parseComponents(tmpl); err != nil {
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

// scrollAreaCell bounds a many-to-many cell in a tabular inline. A comma-
// joined run of links wraps in a table cell: three roles made the row
// three lines tall and pushed its own actions off the edge. A ScrollArea
// keeps row height a function of the table rather than of whichever record
// has the most relations. Only many-to-many; every other field is a single
// value that either fits or is worth wrapping.
func scrollAreaCell(field core.Field, value template.HTML) template.HTML {
	if field.Type != core.FieldTypeManyToMany {
		return value
	}
	return template.HTML(`<span class="`+mustUI("scroll-area", "x")+`">`) + value + template.HTML(`</span>`)
}

type columnHeader struct {
	Label string
	// Sortable is false for a column outside SortableFields: the header
	// renders as plain text, with no menu and no URLs.
	Sortable  bool
	NextSort  string
	Indicator string
	// Explicit Asc/Desc choices rather than a link that cycles, so a click's
	// effect is knowable before making it. Direction is "asc", "desc", or ""
	// when this column isn't the sort column.
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

type filterHidden struct {
	Name  string
	Value string
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
	// Current is this filter's raw value on this request. The badge
	// counts on it rather than on a Selected choice: a custom date range
	// and a combobox selection are both real values that match no
	// declared choice, and counting choices treated those lists as
	// unfiltered.
	Current string
	// Kind is core.FilterKind as a plain string: "" for a link list,
	// "daterange" for the two date inputs, "relation" for the combobox.
	// html/template compares strings cleanly and typed constants badly.
	Kind string
	// Prefilled into the range inputs when the current value is a range
	// rather than a preset. Both empty otherwise.
	RangeFrom string
	RangeTo   string
	// The GET form an input-bearing control submits: the list's own path,
	// plus every other parameter as a hidden field, so submitting
	// reproduces the list it was opened from with one thing changed.
	FormAction string
	Hidden     []filterHidden
	// A relation filter whose field is in AutocompleteFields renders the
	// lookup-backed combobox instead of a link list -- the same
	// declaration, and the same control, the form uses.
	UsesCombobox  bool
	LookupURL     string
	SelectedPK    string
	SelectedLabel string
}

// actionInfo is an Action's template-facing shape --
// leaves out Handler/Permission since templates only need enough to
// render the action bar / record-action buttons and post to its route.
type actionInfo struct {
	Name    string
	Label   string
	Confirm string
	// Preview is set on delete_selected when the ModelAdmin previews
	// deletes: the server's confirmation page replaces the modal.
	Preview bool
}

// actionInfos describes actions for the templates. The caller passes them
// already resolved for its page: core.ActionsForList or core.ActionsForDetail.
func actionInfos(modelAdmin core.ModelAdmin, actions []core.Action) []actionInfo {
	previews := core.PreviewsDeletes(modelAdmin)
	out := make([]actionInfo, 0, len(actions))
	for _, a := range actions {
		info := actionInfo{Name: a.Name, Label: a.Label, Confirm: a.Confirm}
		if previews && a.Name == core.DeleteSelectedName {
			info.Confirm, info.Preview = "", true
		}
		out = append(out, info)
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
	// ActiveFilterCount badges the Filters trigger, so the panel says how
	// much it hides without being opened. Counted here because html/template
	// has no arithmetic. Search is excluded: it has its own visible box.
	ActiveFilterCount int
	// The panel's range form field names, so the template never spells a
	// reserved parameter itself.
	RangeForField  string
	RangeFromField string
	RangeToField   string
	Ordering       string
	ExportQuery    string
	Actions        []actionInfo
	Permissions    permissions
	Reorderable    bool
	// PreviewsDeletes makes the row's Delete a link to the delete page,
	// which is where a preview has anything to say.
	PreviewsDeletes bool
	// ListQuery is "?_list=<this list>" for the pages reached from here,
	// so they lead back into the list as it was left. Empty when the
	// ModelAdmin has preserve_filters off.
	ListQuery string
}

func (r *Renderer) buildListData(
	principal *core.Principal,
	csrfToken string,
	modelAdmin core.ModelAdmin,
	page core.Page,
	req core.ListRequest,
	perms permissions,
	relationPermissions map[string]bool,
	messages []flashMessage,
) listData {
	slug := modelAdmin.Slug()
	listToken := ""
	if modelAdmin.PreservesFilters() {
		listToken = listURL(r.basePath, slug, req, listURLOpts{Page: req.Page})
	}
	listQuery := ""
	if listToken != "" {
		listQuery = "?" + core.ListTokenField + "=" + url.QueryEscape(listToken)
	}
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
			Label: field.Label, Sortable: core.IsSortable(modelAdmin, name),
			NextSort: nextSort, Indicator: indicator,
			Direction: direction,
			AscURL:    listURL(r.basePath, slug, req, listURLOpts{Ordering: name, HasOrder: true}),
			DescURL:   listURL(r.basePath, slug, req, listURLOpts{Ordering: "-" + name, HasOrder: true}),
		})
	}

	rows := make([]listRow, 0, len(page.Items))
	for _, obj := range page.Items {
		pk := modelAdmin.GetPK(obj)
		recordURL := fmt.Sprintf("%s/%s/%v", r.basePath, slug, pk) + listQuery
		cells := make([]template.HTML, 0, len(modelAdmin.ListDisplay()))
		for _, name := range modelAdmin.ListDisplay() {
			field, _ := modelAdmin.Field(name)
			cell := r.fieldValueHTML(relationPermissions, field, field.GetValue(obj), modelAdmin.EmptyValue())
			if perms.CanView && core.LinksToRecord(modelAdmin, name) {
				cell = linkedCell(cell, recordURL)
			}
			cells = append(cells, cell)
		}
		rows = append(rows, listRow{PK: pk, Cells: cells})
	}

	// Each choice is a link, not a <select> option, so each needs its own URL
	// carrying search/ordering/other filters and changing only the one it
	// represents. Page resets to 1, as search and sort do.
	filterControls := make([]filterControl, 0, len(modelAdmin.Filters()))
	for _, filter := range modelAdmin.Filters() {
		current := req.Filters[filter.Name()]

		kind := ""
		if control, declares := filter.(core.FilterControl); declares {
			kind = string(control.ControlKind())
		}

		// A relation filter cannot enumerate its own values -- it reaches
		// neither the registry nor the principal -- so the adapter sources
		// them here and appends them after the filter's own "All".
		var sourced []filterChoice
		usesCombobox, lookupURL, selectedPK, selectedLabel := false, "", "", ""
		if kind == string(core.FilterKindRelation) {
			field, hasField := modelAdmin.Field(filter.Name())
			if !hasField {
				continue // nothing to filter on
			}
			viewable := false
			if autocompleteFields(modelAdmin)[filter.Name()] {
				// The whole point of AutocompleteFields is never loading
				// the target's queryset into the page, so the choices are
				// not sourced at all -- /lookup answers as the reader
				// types. The target still has to be viewable.
				usesCombobox, viewable, lookupURL, selectedPK, selectedLabel =
					relationFilterCombobox(r.admin, principal, modelAdmin, field, current, r.basePath)
			} else {
				sourced, viewable = relationFilterChoices(r.admin, principal, modelAdmin, field)
			}
			if !viewable {
				// Dropped, not emptied -- see relationFilterChoices.
				continue
			}
		}

		pairs := filter.ChoicesWithLabels()
		for _, sourcedChoice := range sourced {
			pairs = append(pairs, [2]string{sourcedChoice.Value, sourcedChoice.Label})
		}
		choices := make([]filterChoice, 0, len(pairs))
		for _, pair := range pairs {
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
		rangeFrom, rangeTo := "", ""
		if kind == string(core.FilterKindDateRange) {
			if from, to, isRange := core.DateFilterRangeValues(current); isRange {
				rangeFrom, rangeTo = from, to
			}
		}
		filterControls = append(filterControls, filterControl{
			Name: filter.Name(), Label: filter.Label(), Choices: choices,
			Active:   active,
			ClearURL: listURL(r.basePath, slug, req, listURLOpts{Filters: others, HasFilters: true}),

			Current:    current,
			Kind:       kind,
			RangeFrom:  rangeFrom,
			RangeTo:    rangeTo,
			FormAction: listPath(r.basePath, slug),
			Hidden:     filterFormHidden(req, filter.Name()),

			UsesCombobox:  usesCombobox,
			LookupURL:     lookupURL,
			SelectedPK:    selectedPK,
			SelectedLabel: selectedLabel,
		})
	}

	activeFilterCount := 0
	for _, control := range filterControls {
		if control.Current != "" {
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
		pageBase:        r.pageBase(principal, csrfToken, r.t(modelAdmin.VerboseName()), r.t(modelAdmin.VerboseName()), "resource:"+slug, r.listBreadcrumbs(modelAdmin), messages),
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
		RangeForField:     core.RangeForField,
		RangeFromField:    core.RangeFromField,
		RangeToField:      core.RangeToField,
		Ordering:          req.Ordering,
		ExportQuery:       exportQuery(req),
		Actions:           actionInfos(modelAdmin, core.ActionsForList(modelAdmin)),
		Permissions:       perms,
		Reorderable:       modelAdmin.Reorderable(),
		PreviewsDeletes:   core.PreviewsDeletes(modelAdmin),
		ListQuery:         listQuery,
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

// filterChoiceURL is one link in the filter sidebar: everything else
// carries over unchanged, filterName is set to value, and an empty value
// clears that filter. Page is omitted, resetting to 1.
// listPath is the list's own URL with no query -- a GET form's action,
// since the form supplies the query itself.
func listPath(basePath, slug string) string {
	return basePath + "/" + slug
}

// filterFormHidden is every list parameter except the one the form is
// about, as hidden fields. Without them, submitting the form would drop
// the reader's search, sort and other filters.
func filterFormHidden(req core.ListRequest, exclude string) []filterHidden {
	hidden := make([]filterHidden, 0, len(req.Filters)+2)
	if req.Search != "" {
		hidden = append(hidden, filterHidden{Name: "search", Value: req.Search})
	}
	if req.Ordering != "" {
		hidden = append(hidden, filterHidden{Name: "sort", Value: req.Ordering})
	}
	// Page is deliberately dropped: narrowing a list returns to page 1,
	// exactly as the choice links already do.
	names := make([]string, 0, len(req.Filters))
	for name := range req.Filters {
		if name != exclude {
			names = append(names, name)
		}
	}
	sort.Strings(names) // stable output, so the tests can assert on it
	for _, name := range names {
		hidden = append(hidden, filterHidden{Name: "filter[" + name + "]", Value: req.Filters[name]})
	}
	return hidden
}

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

// listURLOpts overrides individual parameters of the current list request.
// Each override pairs with a Has* flag: without one, "clear the search"
// and "leave it alone" would both be the zero value.
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

// listURL carries over every parameter of the current request except those
// overridden. Every control on the list page is a link needing the same
// "keep what's there, change one thing" rule, so building it once here
// keeps the templates free of query-string assembly.
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

func (r *Renderer) RenderList(principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, page core.Page, req core.ListRequest, perms permissions, relationPermissions map[string]bool, messages []flashMessage) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "list", r.list)
	if err != nil {
		return "", err
	}
	data := r.buildListData(principal, csrfToken, modelAdmin, page, req, perms, relationPermissions, messages)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (r *Renderer) RenderListFragment(principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, page core.Page, req core.ListRequest, perms permissions, relationPermissions map[string]bool) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "list", r.list)
	if err != nil {
		return "", err
	}
	data := r.buildListData(principal, csrfToken, modelAdmin, page, req, perms, relationPermissions, nil)
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "content", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// A field's HelpText deliberately does not appear here: it explains how
// to fill a field in, which is a question the form answers and the
// detail page does not ask. See ui/field.html.
type detailField struct {
	Label string
	Value template.HTML
}

// historyEntry is one audit entry, formatted for display.
type historyEntry struct {
	When string
	Who  string
	What string
}

type detailData struct {
	pageBase
	Slug           string
	PK             any
	Fields         []detailField
	Actions        []actionInfo
	Permissions    permissions
	InlineSections []inlineSectionData
	WideBody       bool
	// History is empty unless the AuditLogger also reads back
	// (core.AuditReader); a write-only logger is a fine arrangement when the
	// log's consumer is elsewhere.
	History []historyEntry
}

// historyFor returns the record's recent audit entries, or nil when no
// logger is configured or the one configured cannot read back.
func (r *Renderer) historyFor(ctx context.Context, modelAdmin core.ModelAdmin, obj any) []historyEntry {
	reader, ok := r.admin.AuditLogger.(core.AuditReader)
	if !ok {
		return nil
	}
	entries, err := reader.History(ctx, modelAdmin.Slug(), modelAdmin.GetPK(obj), historyLimit)
	if err != nil {
		// A history panel is not worth failing a page render over.
		log.Printf("polyadmin: audit history unavailable for %s: %v", modelAdmin.Slug(), err)
		return nil
	}
	out := make([]historyEntry, 0, len(entries))
	for _, entry := range entries {
		who := r.t("system")
		if entry.Principal != nil {
			if entry.Principal.DisplayName != "" {
				who = entry.Principal.DisplayName
			} else if entry.Principal.ID != nil {
				who = fmt.Sprint(entry.Principal.ID)
			}
		}
		out = append(out, historyEntry{
			When: entry.At.Format("2006-01-02 15:04"),
			Who:  who,
			What: r.auditAction(entry.Action),
		})
	}
	return out
}

// auditAction is how the History panel names what happened. The
// framework's own verbs are translated; anything else is the name of the
// Action that ran, an identifier stored in the log, and stays as it is.
func (r *Renderer) auditAction(action string) string {
	switch action {
	case core.AuditCreate:
		return r.t("create")
	case core.AuditUpdate:
		return r.t("update")
	case core.AuditDelete:
		return r.t("delete")
	default:
		return action
	}
}

// historyLimit caps the detail page's History panel. It is a summary of
// recent activity, not an audit browser -- a logger that wants the full
// trail exposed can surface it wherever it already lives.
const historyLimit = 10

// ctx is threaded through for the audit history lookup, which is a
// real read against the host's log rather than page data already in
// hand -- it deserves the request's cancellation like any other.
func (r *Renderer) RenderDetail(ctx context.Context, principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, obj any, perms permissions, relationPermissions map[string]bool, messages []flashMessage, listToken string) (string, error) {
	fields := make([]detailField, 0, len(modelAdmin.DetailFields()))
	for _, name := range modelAdmin.DetailFields() {
		field, _ := modelAdmin.Field(name)
		fields = append(fields, detailField{
			Label: field.Label,
			Value: r.fieldValueHTML(relationPermissions, field, field.GetValue(obj), modelAdmin.EmptyValue()),
		})
	}
	inlineSections, err := r.buildInlineSections(principal, modelAdmin, obj, "readonly", "", nil, nil, nil)
	if err != nil {
		return "", err
	}
	data := detailData{
		pageBase:       r.pageBase(principal, csrfToken, r.t(modelAdmin.VerboseName()), r.t(modelAdmin.VerboseName()), "resource:"+modelAdmin.Slug(), r.detailBreadcrumbs(modelAdmin, obj, listToken), messages),
		Slug:           modelAdmin.Slug(),
		PK:             modelAdmin.GetPK(obj),
		Fields:         fields,
		Actions:        actionInfos(modelAdmin, core.ActionsForDetail(modelAdmin)),
		History:        r.historyFor(ctx, modelAdmin, obj),
		Permissions:    perms,
		InlineSections: inlineSections,
		WideBody:       wideBody(inlineSections),
	}
	data.ListToken = listToken
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
	Fieldsets      []fieldsetData
	InlineSections []inlineSectionData
	WideBody       bool
	// AllowSaveAs adds "Save as new" beside Save, on an edit form only --
	// there is nothing to copy from on a create form.
	AllowSaveAs bool
	// Prepopulated is the create form's field-filling map as JSON, read by
	// the behaviour in theme.html. Empty on an edit form and when the
	// ModelAdmin declares none.
	Prepopulated string
}

// fieldsetData is one rendered group of form inputs. A group with an
// empty Title renders bare -- see form.html -- which is what keeps the
// default (undeclared) single-group case looking like a flat form.
type fieldsetData struct {
	Title       string
	Description string
	Collapsed   bool
	Inputs      []template.HTML
}

func (r *Renderer) RenderForm(
	principal *core.Principal,
	csrfToken string,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
	listToken string,
) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "form", r.form)
	if err != nil {
		return "", err
	}
	return r.executeForm(tmpl, "base", principal, csrfToken, modelAdmin, obj, submitted, errs, relationOptions, listToken)
}

func (r *Renderer) RenderFormFragment(
	principal *core.Principal,
	csrfToken string,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
	listToken string,
) (string, error) {
	tmpl, err := r.contentTemplate(modelAdmin, "form", r.form)
	if err != nil {
		return "", err
	}
	return r.executeForm(tmpl, "content", principal, csrfToken, modelAdmin, obj, submitted, errs, relationOptions, listToken)
}

func (r *Renderer) executeForm(
	tmpl *template.Template,
	name string,
	principal *core.Principal,
	csrfToken string,
	modelAdmin core.ModelAdmin,
	obj any,
	submitted map[string]any,
	errs map[string][]string,
	relationOptions map[string]*relationFieldOptions,
	listToken string,
) (string, error) {
	title, action := r.t("Create %s", r.t(modelAdmin.VerboseName())), fmt.Sprintf("%s/%s/create", r.basePath, modelAdmin.Slug())
	if obj != nil {
		title = r.t("Edit %s", r.t(modelAdmin.VerboseName()))
		action = fmt.Sprintf("%s/%s/%v/edit", r.basePath, modelAdmin.Slug(), modelAdmin.GetPK(obj))
	}

	// Grouped rather than one flat list: Fieldsets() always yields at
	// least one group, so this is the single path for both the declared
	// and the undeclared case.
	sets := modelAdmin.Fieldsets()
	fieldsets := make([]fieldsetData, 0, len(sets))
	for _, set := range sets {
		group := fieldsetData{Title: set.Title, Description: set.Description, Collapsed: set.Collapsed}
		for _, fieldName := range set.Fields {
			field, _ := modelAdmin.Field(fieldName)
			value, ok := submitted[fieldName]
			if !ok && obj != nil {
				value = field.GetValue(obj)
			}
			input, err := r.formInputHTML(r.basePath, field, value, errs[fieldName], relationOptions[fieldName],
				modelAdmin.IsReadOnly(fieldName, obj))
			if err != nil {
				return "", err
			}
			group.Inputs = append(group.Inputs, input)
		}
		fieldsets = append(fieldsets, group)
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
		pageBase:    r.pageBase(principal, csrfToken, title, title, "resource:"+modelAdmin.Slug(), r.formBreadcrumbs(modelAdmin, obj, listToken), nil),
		VerboseName: modelAdmin.VerboseName(),
		FormAction:  action,
		// The edit form offers Delete, so it needs the detail page's permission
		// map: otherwise the button renders for a principal the route then
		// rejects.
		Permissions:    computePermissions(r.admin, principal, modelAdmin, obj),
		Fieldsets:      fieldsets,
		InlineSections: inlineSections,
		WideBody:       wideBody(inlineSections),
		AllowSaveAs:    modelAdmin.AllowsSaveAs() && obj != nil,
		Prepopulated:   prepopulatedJSON(modelAdmin, obj),
	}
	data.ListToken = listToken
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

// Cells arrive pre-rendered, the same convention as formData.Inputs. Which
// builder runs is decided here by inline.Layout rather than in the
// template, so inline.html's stacked and tabular defines stay a uniform
// `{{range .Cells}}{{.}}{{end}}`.

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

// wideBody reports whether any inline section is tabular. A table needs
// more than the max-w-xl a column of form fields wants -- squeezing one in
// is what clipped its row actions -- so such a page gets ui "page" "body-
// wide".
func wideBody(sections []inlineSectionData) bool {
	for _, section := range sections {
		if section.Layout == core.InlineLayoutTabular {
			return true
		}
	}
	return false
}

type inlineSectionData struct {
	Slug      string
	Label     string
	Layout    string // core.InlineLayoutStacked | core.InlineLayoutTabular
	Mode      string // "placeholder" | "edit" | "readonly"
	CanChange bool
	CanDelete bool
	// Columns holds header labels for the tabular layout only; column i
	// matches Rows[*].Cells[i]. Stacked cells carry their own labels.
	Columns []inlineColumn
	Rows    []inlineRowData
	AddRow  *inlineAddRowData // nil unless Mode == "edit" && the principal has the child's own create permission
	// Refusal explains a Remove the child's DeletePreview blocked; nil
	// every other time the section is rendered.
	Refusal *deletePreviewView
}

// buildInlineSections builds one inlineSectionData per viewable Inline.
// Form, detail, and fragment rendering all go through it, so the three can
// never drift apart.
//
// The redisplay* arguments carry a failed mutation's submitted values back
// into the one row being redisplayed (the add-row if redisplayPK is nil),
// and are zero everywhere but RenderInlineFragment.
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
		childPerms := computePermissions(r.admin, principal, childAdmin, nil)

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
						cells = append(cells, r.inlineTableCellHTML(r.basePath, field, value, errs[name], relOptions[name]))
					} else {
						input, err := r.formInputHTML(r.basePath, field, value, errs[name], relOptions[name],
							childAdmin.IsReadOnly(name, nil))
						if err != nil {
							return nil, err
						}
						cells = append(cells, input)
					}
				}
			} else {
				detailURL := fmt.Sprintf("%s/%s/%v", r.basePath, inline.Child, childPK)
				linkColumn := pkColumn(detailNames)
				for _, name := range detailNames {
					field, _ := childAdmin.Field(name)
					valueHTML := r.fieldValueHTML(relPerms, field, field.GetValue(child), childAdmin.EmptyValue())
					if inline.Layout == core.InlineLayoutTabular {
						cell := scrollAreaCell(field, valueHTML)
						// The primary key's cell opens the record, so the row
						// needs no column of its own for a View link.
						// linkedCell leaves a cell that is already a link --
						// a relation -- alone.
						if name == linkColumn {
							cell = linkedCell(cell, detailURL)
						}
						cells = append(cells, cell)
					} else {
						cells = append(cells, inlineDetailRowHTML(r.t(field.Label), valueHTML))
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
					cells = append(cells, r.inlineTableCellHTML(r.basePath, field, value, errs[name], relOptions[name]))
				} else {
					input, err := r.formInputHTML(r.basePath, field, value, errs[name], relOptions[name],
						childAdmin.IsReadOnly(name, nil))
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

// RenderInlineFragment renders one inline section standalone: the response
// body for the inline create/update/delete routes, swapped into
// #inline-{child_slug} by htmx.
func (r *Renderer) RenderInlineFragment(
	principal *core.Principal, modelAdmin core.ModelAdmin, obj any, inline core.Inline,
	redisplayPK any, redisplayData map[string]any, redisplayErrs map[string][]string,
) (string, error) {
	return r.renderInlineSection(principal, modelAdmin, obj, inline, redisplayPK, redisplayData, redisplayErrs, nil)
}

// renderInlineSection is RenderInlineFragment plus the refusal an inline
// Remove blocked by the child's DeletePreview shows above the rows.
func (r *Renderer) renderInlineSection(
	principal *core.Principal, modelAdmin core.ModelAdmin, obj any, inline core.Inline,
	redisplayPK any, redisplayData map[string]any, redisplayErrs map[string][]string,
	refusal *deletePreviewView,
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
	section.Refusal = refusal
	var buf bytes.Buffer
	if err := r.inlineFragment.ExecuteTemplate(&buf, "content", section); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type deleteData struct {
	pageBase
	VerboseName string
	ObjectLabel string
	Preview     deletePreviewView
}

func (r *Renderer) RenderDelete(principal *core.Principal, csrfToken string, modelAdmin core.ModelAdmin, obj any, preview core.ResolvedDeletePreview, listToken string) (string, error) {
	title := r.t("Delete %s", r.t(modelAdmin.VerboseName()))
	data := deleteData{
		pageBase:    r.pageBase(principal, csrfToken, title, title, "resource:"+modelAdmin.Slug(), r.deleteBreadcrumbs(modelAdmin, obj, listToken), nil),
		VerboseName: modelAdmin.VerboseName(),
		ObjectLabel: objectLabel(modelAdmin, obj),
		Preview:     r.deletePreviewView(preview),
	}
	data.ListToken = listToken
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

// widgetIcons maps a widget's Template() to the icon shown in its
// dashboard card badge.
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

// widgetTemplate returns the set defining widget.Template()'s name: the
// pre-built one for a built-in widget, otherwise a resolved and cached set
// from WithTemplateDirs. A custom widget's file must define a block named
// after its own Template() value. Widgets aren't tied to a ModelAdmin, so
// per-resource overrides can't cover them.
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
		tmpl, err := parseComponents(template.New(path.Base(name)).Funcs(r.funcs))
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

// renderWidgetBody executes one widget's template against its GetData. A
// core.Container gets its panels rendered first and receives them as HTML:
// html/template cannot execute a name known only at runtime, so the
// recursion happens here rather than inside tabs.html.
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

func (r *Renderer) RenderDashboard(principal *core.Principal, csrfToken string, dashboard core.Dashboard, widgets []core.Widget) (string, error) {
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
	title = r.t(title)
	// A single active crumb -- since base.html has no separate <h1>,
	// this is the only page-title element the dashboard gets.
	data := dashboardData{pageBase: r.pageBase(principal, csrfToken, title, title, "", []breadcrumb{{Label: title, Active: true}}, nil), Widgets: rendered}
	var buf bytes.Buffer
	if err := r.dashboard.ExecuteTemplate(&buf, "base", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type pageData struct {
	pageBase
	Page core.AdminPage
	Data any // handler-supplied extra template data
}

// PageTemplate resolves and caches an AdminPage's template the way
// contentTemplate resolves an override. There is no framework default to
// fall back on: a page template is always application-supplied.
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
		tmpl, err := template.New(path.Base(templateName)).Funcs(r.funcs).
			ParseFS(coretemplates.FS, layoutFiles...)
		if err != nil {
			return nil, err
		}
		if tmpl, err = parseComponents(tmpl); err != nil {
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

// RenderPage renders an AdminPage's template inside the shared layout.
// data reaches it as .Data, alongside .Page.
func (r *Renderer) RenderPage(principal *core.Principal, csrfToken string, page core.AdminPage, templateName string, data any, messages []flashMessage) (string, error) {
	tmpl, err := r.PageTemplate(templateName)
	if err != nil {
		return "", err
	}
	label := r.t(page.Label)
	breadcrumbs := append(r.categoryBreadcrumb(page.Category), breadcrumb{Label: label, Active: true})
	pd := pageData{
		pageBase: r.pageBase(principal, csrfToken, label, label, "page:"+page.Path, breadcrumbs, messages),
		Page:     page,
		Data:     data,
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", pd); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// loginData is deliberately not a pageBase: nothing on this page comes
// from the admin shell. There is no principal (that is the point), no
// nav to build, and no breadcrumb trail to sit in.
type loginData struct {
	CSRFToken   string
	BasePath    string
	SiteTitle   string
	SiteLogoURL string
	// Identifier is echoed back after a failed attempt so a mistyped
	// password does not cost the email as well.
	Identifier string
	// Error is shown as a destructive alert, Notice as a plain one --
	// "those credentials are wrong" versus "you have been signed out".
	Error  string
	Notice string
}

func (r *Renderer) RenderLogin(csrfToken, identifier, errorMessage, notice string) (string, error) {
	var buf bytes.Buffer
	data := loginData{
		CSRFToken:   csrfToken,
		BasePath:    r.basePath,
		SiteTitle:   r.t(r.admin.SiteTitle),
		SiteLogoURL: r.admin.SiteLogoURL,
		Identifier:  identifier,
		Error:       errorMessage,
		Notice:      notice,
	}
	if err := r.login.ExecuteTemplate(&buf, "login", data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

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
