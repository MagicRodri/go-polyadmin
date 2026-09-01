# Templates

How rendering works, and how to override a template.

## Three-level override resolution

`Mount(router, admin, basePath, fiberadapter.WithTemplateDirs("templates"))`
adds an on-disk search directory, checked before the framework's own
`go:embed`-baked templates. Per view, `contentTemplate` resolves, in
order:

1. An explicit override — `BaseModelAdmin.ListTemplate`/`DetailTemplate`/
   `FormTemplate`/`DeleteTemplate` (or `TemplateOverride(view)`, if you
   implement `core.ModelAdmin` without embedding `BaseModelAdmin`).
2. A resource-specific template — `admin/resource/{slug}/{view}.html`.
3. The framework default — `admin/resource/{view}.html`.

```go
fiberadapter.Mount(group, admin, "/admin", fiberadapter.WithTemplateDirs("templates"))
```

```
templates/
└── admin/
    └── resource/
        └── users/
            └── list.html    # only replaces the Users list view
```

`html/template` needs named blocks to layer content into `base.html` —
an override file **must** define a `{{define "content"}}...{{end}}`
block (that's what gets executed into the page; `base.html` itself
always comes from the framework, never from an override). Applications
that never call `WithTemplateDirs` and never set a `*Template` field pay
no cost for any of this — the check is two cheap comparisons that fall
straight through to the pre-built framework template on every request.

An override doesn't have to start from scratch. The partials the
framework's own page templates are built from are parsed into the same
set, so a mostly-standard list page can keep the stock region and add
around it:

```gotemplate
{{define "content"}}
<p class="mb-4">Only active accounts are shown.</p>
{{template "listContent" .}}
{{end}}
```

`listContent`, `search` and `formWrapper` are all available this way.

Custom dashboard widgets get the same treatment through the same
option: a widget whose `Template()` name isn't one of the built-ins is
looked up across the `WithTemplateDirs` directories the same way, and
must define a block named after its own `Template()` value (not
`"content"` — widgets don't have a base layout of their own to layer
into, they're inserted directly into the dashboard's widget grid). See
[`dashboard.md`](dashboard.md#custom-widgets).

## Custom admin page templates

A custom `AdminPage` (see [`routing.md`](routing.md#custom-admin-pages))
renders its own template, not a framework-owned one — there's no
`admin/page.html` default to fall back to, since the whole point is
application-specific markup.

`Renderer.PageTemplate`/`RenderPage` resolve the template from
`WithTemplateDirs` directories only — unlike a `ModelAdmin` view,
there's no framework-default fallback to check first, so a page
template that isn't found under any configured directory is an error.
It must define a `{{define "content"}}` block, the same convention a
`ModelAdmin` template override uses:

```gotemplate
{{define "content"}}
<h1>{{.Page.Label}}</h1>
{{end}}
```

`PageContext.Render`'s `data` argument is available in the template as
`.Data`.

## The `admin/` template namespace

The framework's own templates live under a directory literally named
`admin/` (`templates/admin/*.html`, embedded with `go:embed`) — this is
a template *lookup* namespace, unrelated to the project's own name;
Django keeps the same convention for its own admin templates regardless
of how a site is branded. `template.ExecuteTemplate(&buf, "base", data)`
referencing `admin/base.html` is internal plumbing, not user-facing.

## The template tree

```
admin/
├── base.html                     # the shell (sidebar, breadcrumbs, flash)
├── theme.html                    # design tokens, Tailwind config, Alpine/HTMX
├── login.html                    # the one page outside the shell
├── dashboard.html
├── resource/                     # the four generated views, and where
│   ├── list.html                 #   a resource's own override goes:
│   ├── detail.html               #   admin/resource/{slug}/{view}.html
│   ├── form.html
│   └── delete.html
├── components/                   # partials the pages are assembled from
│   ├── list_content.html         #   the swappable #resource-list region
│   ├── search.html
│   ├── form_wrapper.html
│   ├── inline.html
│   ├── inline_fragment.html
│   ├── lookup_results.html
│   ├── toasts.html
│   ├── action_confirm_modal.html
│   ├── csrf-field.html
│   └── ui/                       # the shadcn/ui ports
└── widgets/                      # dashboard widgets
```

`resource/list.html` and `resource/form.html` are deliberately thin —
each is a shim defining `content` over `components/list_content.html` /
`components/form_wrapper.html`, because those inner regions are also
rendered on their own as HTMX fragments. Keeping them in separate files
means an override can invoke one by name instead of copying its markup.

Icons and read-only field values have no template of their own: both
are built in Go (`fiber/icons.go`, `fiber/render_helpers.go`), which is
the same split the `fiber` package's doc comment describes — field and
form HTML is assembled in Go for tighter control over escaping with
`html/template`.

## What a template gets

The `Renderer` builds a context struct per view — `listData`,
`detailData`, `formData`, `deleteData`, `dashboardData`, each embedding
a shared `pageBase` (`fiber/render.go`) — carrying: the resource's
rows/fields for that view, computed per-request permissions (so
Edit/Delete/Create/Export controls are omitted server-side when
unavailable, not just hidden with CSS), breadcrumbs, site title/logo,
and any pending flash messages. The template layer never receives the
raw `ModelAdmin` object's storage internals — only what the view
actually needs to render.

## Styling

The design system is [shadcn/ui](https://ui.shadcn.com), hand-ported to
Alpine.js — see [`components.md`](components.md) for the full component
list and the porting rationale. In short:

- **Colors are tokens, never literals.** `bg-background`,
  `text-muted-foreground`, `border-input` and friends resolve through
  CSS variables declared in `admin/theme.html`. No template names a
  Tailwind shade like `neutral-500`, which is what makes the admin
  themeable and gives it dark mode for free.
- **Class lists come from `ui(...)`**, the server-side stand-in for
  shadcn's `class-variance-authority`: `{{ui "button" "outline" "size-sm"}}`
  composes a base with a variant and a size, while `{{ui "table" "th"}}`
  resolves a sub-component's own list. The registry lives in
  `fiber/ui.go`.
- **Radix is replaced by Alpine, not shipped.** Focus trapping is
  `x-trap`, portals are `x-teleport`, popover positioning is `x-anchor`,
  collapse is `x-collapse`. There is no React and no Radix runtime.
- **Controls that gain nothing from a listbox stay native** — checkbox,
  radio, range, `<input type="date">`, and `<select multiple>`
  (manytomany) are styled to match shadcn rather than replaced by a
  Radix-style widget. A single-value `<select>` (enum fields, a plain
  foreignkey/onetoone) is the one exception: it's `ui/select.html`, a
  real trigger+listbox port with a hidden input carrying the posted
  value, matching the bulk-actions listbox and the relation combobox's
  own reasoning. The other genuinely Alpine-driven ports are the
  relation combobox, the date picker's calendar popover, the dialog,
  the dropdown menu, the sheet, and the toasts.
- **A few components predate the port** (the toast queue, the confirm
  dialog) and came from [PinesUI](https://devdojo.com/pines); they were
  restyled onto the same tokens rather than rewritten.

Tailwind, Alpine (plus its focus/collapse/anchor plugins), and HTMX are
all CDN-loaded from `admin/theme.html` — there is no frontend build step
to run.

### Dark mode

`admin/theme.html` resolves the theme in a synchronous inline script
before first paint (so a dark reload doesn't flash light), toggling a
`dark` class on `<html>`, and exposes an `Alpine.store('theme')` that
the header's toggle button drives. The preference persists in
`localStorage` under `polyadmin-theme`; with nothing stored it follows
`prefers-color-scheme`.

To restyle the whole admin, override `admin/theme.html` and change the
CSS variables — nothing else needs to know.
