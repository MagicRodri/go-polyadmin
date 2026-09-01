# Architecture

go-polyadmin is a Django-admin-style admin framework for
[Fiber](https://gofiber.io). It owns presentation — routes, forms,
tables, permissions checks, HTML — and owns no storage: your
`ModelAdmin` implements the lifecycle hooks against whatever database
or service you already have. This document explains how the pieces fit
together; for any one concept in depth, see the other files in this
directory.

## The four core objects

**`Field`** describes one model attribute's admin presentation: its
type (string, integer, boolean, date, email, foreign key, ...),
whether it's required/readonly/disabled, and how to read a value off
an arbitrary object and coerce a submitted form value back into one.
Fields don't know about HTTP or HTML — they're the data-level contract
everything else builds on. See [`model-admin.md`](model-admin.md).

**`ModelAdmin`** is one resource: which model it administers, which
fields appear in the list/detail/form views, search/filter/ordering
config, the CRUD lifecycle hooks (`GetQueryset`, `GetObject`, `Create`,
`Update`, `Delete`), and optional Actions and autocomplete relation
fields. A `ModelAdmin` never touches your database directly — it's an
abstract contract; your implementation supplies the lifecycle hooks
against whatever storage you actually have. See
[`model-admin.md`](model-admin.md).

**`Admin`** is the site: a registry of `ModelAdmin`s keyed by slug,
plus the optional `Dashboard`, `Authenticator`, and `Authorizer` for
the whole site, and branding (`SiteTitle`, `SiteLogoURL`). It owns no
HTTP concerns either — mounting it onto a real router is the adapter's
job.

**The adapter** (`fiber`) is the only layer that knows about HTTP.
`Mount(router, admin, basePath)` walks the `Admin`'s registry and
builds routes for each viewable/creatable/updatable/deletable/
exportable `ModelAdmin`, wires in the `Authenticator`/`Authorizer` on
every route, and renders responses through a `Renderer`. See
[`routing.md`](routing.md).

## Request flow

A request for `GET /admin/users` (list view) goes:

1. The adapter's route handler authenticates the request
   (`Authenticator.Authenticate` → a `*Principal` or `nil`) and
   authorizes it (`Authorizer.Can(principal, "users.list", modelAdmin)`)
   before anything else runs.
2. `ModelAdmin.GetQueryset()` returns the resource's base collection —
   or, if the `ModelAdmin` implements `core.ListQuerier`, `ListPage`
   resolves search, filters, ordering and the page window in one go
   against the data source itself.
3. The query pipeline (`core/query.go`) applies search, declared
   `Filter`s, and ordering from the query string; `Paginate` slices the
   result.
4. Per-row/detail relation fields are resolved against the
   `Authorizer` too — a relation only renders as a clickable link if
   the principal can view the target resource, otherwise it falls back
   to plain text.
5. The `Renderer` picks a template (see [`templates.md`](templates.md)
   for the override order), builds its context, and returns HTML — a
   full page normally, or just the `#resource-list` fragment when the
   request came from an HTMX-driven search/filter/sort/pagination
   interaction.

Every other route (detail, create, edit, delete, lookup, actions,
export) follows the same authenticate → authorize → do the thing →
render shape.

## An idiomatic Go API

The API is built out of the tools Go already gives you rather than
imitating a dynamic framework: functional options
(`core.NewField(..., core.WithRequired())`), struct embedding
(`BaseModelAdmin` embedded, its methods promoted, any one of them
overridable), and interfaces for the extension points
(`Authenticator`, `Authorizer`, `AuditLogger`, `ListQuerier`).

Optional capabilities are expressed as interfaces the framework
type-asserts for, not as methods every implementation must stub out: a
`ModelAdmin` that implements `core.ListQuerier` gets database-side
pagination, one that doesn't keeps working through `GetQueryset`. The
same shape gives an `AuditLogger` an optional read side
(`core.AuditReader`) and an application an optional login page
(`core.LoginBackend`).

Field and form HTML is built in plain Go functions
(`fiber/render_helpers.go`) rather than inside the template files, for
tighter control over escaping with `html/template`.

## Frontend

The admin renders server-side HTML styled with
[shadcn/ui](https://ui.shadcn.com), hand-ported to Alpine.js + Tailwind:
its CSS-variable token system and component markup, with Radix's
behavior reimplemented in Alpine and no React anywhere. Colors resolve
through those variables rather than a literal palette, which is what
makes the admin themeable and dark-mode-capable. HTMX handles
partial-page updates (list search/filter/sort/pagination, and form
validation redisplay). Tailwind, Alpine (plus its focus/collapse/anchor
plugins), and HTMX are all CDN-loaded — there is no frontend build
step.

See [`components.md`](components.md) for the component reference and the
porting rationale, and [`templates.md`](templates.md#styling) for how to
retheme.
