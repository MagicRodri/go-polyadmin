# go-polyadmin

[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![CI](https://github.com/MagicRodri/go-polyadmin/actions/workflows/notify-docs.yml/badge.svg)](https://github.com/MagicRodri/go-polyadmin/actions/workflows/notify-docs.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![GitHub Repo stars](https://img.shields.io/github/stars/MagicRodri/go-polyadmin?style=flat-square)](https://github.com/MagicRodri/go-polyadmin)

A server-rendered operations workspace for Go applications, with a Fiber
adapter. See [`docs/`](docs/) for reference documentation, or visit the
[PolyAdmin documentation site](https://magicrodri.github.io/polyadmin-docs/)
for the shared Go and Python guides.

## What you get from declaring a `ModelAdmin`

Point one at your model (fields, `ListDisplay`, search/filter config,
permissions) and you get:

- Full CRUD with search, filter, sort, and pagination, all swapped in
  via HTMX partials — no full-page reloads
- A dashboard of pluggable widgets: metric, stat (value + trend),
  progress bar, bar chart, donut/pie breakdown, table, activity feed,
  timeline, and tabs (several widgets in one card)
- Sidebar grouping — resources sharing a `category` collapse into one
  collapsible accordion section, e.g. everything CRUD-shaped under
  one "Directory" group
- Custom admin pages (`admin.Route`) for functionality that isn't
  resource CRUD — reports, wizards, internal tools — rendered inside
  the same layout, auth, and sidebar grouping as everything else
- Relation fields rendered as links, or as a searchable shadcn/ui
  Command-style autocomplete backed by a server-side `/lookup` route
  (never dumps the target model's full queryset into the page)
- Image fields with upload handling and preview rendering
- Inline related records (`StackedInline`/`TabularInline`) — a
  parent's create/detail/edit pages can show and manage a child's
	records that point back at it, directly within the parent workflow
- Record and bulk Actions, with a shadcn/ui Dialog confirmation step for
  destructive ones
- Delete previews: the confirmation page says what else a delete takes
  with it, and protected records block it
- List niceties: a date drill-down, restricted sorting, chosen link
  columns, and filters that survive the trip to a record and back
- Authentication/authorization hooks gating every route *and* every
  control the templates render
- CSV and XLSX export
- Internationalisation: per-request locale, language switcher, French
  and Russian
- Built-in favicon support for the admin shell
- Toast notifications for every create/update/delete/action
- Per-resource (and per-widget) template overrides, so an application
  can replace one view's markup without forking the framework

Styling is [shadcn/ui](https://ui.shadcn.com), hand-ported to
Alpine.js + Tailwind — its CSS-variable token system and component
markup, without React or Radix. That gives the admin **dark mode and
themability**: every color resolves through a CSS variable, so restyling
the whole thing is a change to one template. The layout is mobile-first
with a collapsible sidebar (a Sheet below `md`), a focused
right-hand filter panel on wider screens, and breadcrumbs as the page
title. Tailwind/Alpine/HTMX are all CDN-loaded — no frontend build step.
See [`docs/components.md`](docs/components.md).

## Quickstart

```bash
go get github.com/MagicRodri/go-polyadmin                # core
go get github.com/MagicRodri/go-polyadmin/contrib/fiber  # Fiber adapter
go get github.com/MagicRodri/go-polyadmin/contrib/gorm   # GORM model admin
```

Each integration under `contrib/` is its own Go module, so an application
only downloads the dependencies of the adapters it imports. Since
`v0.1.0-beta.4` the Fiber adapter lives at
`github.com/MagicRodri/go-polyadmin/contrib/fiber` (it was
`github.com/MagicRodri/go-polyadmin/fiber`); the package is still named
`fiber`. In the same release `core.ChoiceFilter.Choices` became a slice of
`[2]string` (value, label) pairs; `core.NewChoiceFilter(name, []string)` is
unchanged, and `core.NewChoicePairsFilter` takes labeled pairs.

The first beta release is available as a reproducible Go module version:

```bash
go get github.com/MagicRodri/go-polyadmin@v0.1.0-beta.2
```

Declare a `ModelAdmin` against your own storage and mount it on a
Fiber app:

```go
package main

import (
	"context"
	"log"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/contrib/fiber"

	"github.com/gofiber/fiber/v2"
)

type User struct {
	ID       int
	Email    string
	IsActive bool
}

var users []*User

type UserAdmin struct {
	core.BaseModelAdmin
}

func (a *UserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, len(users))
	for i, u := range users {
		out[i] = u
	}
	return out, nil // must be []any -- see "Adapter contract" below
}

func (a *UserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	for _, u := range users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

func (a *UserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	email, _ := data["Email"].(string)
	isActive, _ := data["IsActive"].(bool)
	u := &User{ID: len(users) + 1, Email: email, IsActive: isActive}
	users = append(users, u)
	return u, nil
}

func (a *UserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	u := obj.(*User)
	u.Email, _ = data["Email"].(string)
	u.IsActive, _ = data["IsActive"].(bool)
	return u, nil
}

func (a *UserAdmin) Delete(ctx context.Context, obj any) error {
	u := obj.(*User)
	for i, existing := range users {
		if existing == u {
			users = append(users[:i], users[i+1:]...)
			break
		}
	}
	return nil
}

func main() {
	userAdmin := &UserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "IsActive"},
			FormFieldNames:   []string{"Email", "IsActive"},
			SearchFieldNames: []string{"Email"},
			DeclaredFields: []core.Field{
				core.NewField("Email", core.FieldTypeEmail, core.WithRequired()),
				core.NewField("IsActive", core.FieldTypeBoolean, core.WithDefault(true)),
			},
		},
	}

	admin := core.New(core.WithModelAdmins(userAdmin))

	app := fiber.New()
	group := app.Group("/admin")
	if err := fiberadapter.Mount(group, admin, "/admin"); err != nil {
		log.Fatal(err)
	}
	log.Fatal(app.Listen(":3000"))
}
```

```bash
go run .
# open http://127.0.0.1:3000/admin
```

That's a full CRUD admin for `User` — search, sort, create, edit,
delete, CSV/XLSX export, all with zero routes or templates of your own.
With no `Authenticator`/`Authorizer` set, every request is allowed by
default (fine for exploring locally, not for anything real) — see
[`docs/authentication.md`](docs/authentication.md) and
[`docs/permissions.md`](docs/permissions.md) before deploying.
For everything else a `ModelAdmin` supports (relations, filters,
actions, a dashboard, exports, delete previews), see
[`docs/model-admin.md`](docs/model-admin.md); for the UI components and
theming, [`docs/components.md`](docs/components.md); and for the rest,
[`docs/`](docs/).

## Running the tests

`./scripts/test.sh` builds, vets and tests every module: the core,
`contrib/fiber`, `contrib/gorm` and `examples/fiber`. `go test ./...` at
the root only covers the core module.

## Releasing

The core and each contrib module are tagged separately. `contrib/fiber/go.mod`
keeps its `replace github.com/MagicRodri/go-polyadmin => ../..` permanently:
Go ignores a dependency's `replace`, so consumers are unaffected, and local
development keeps building the adapter against the core in the same checkout.
What consumers do see is the adapter's `require` on the core, which must name a
published core version.

1. Tag the core (`v0.1.0-beta.4`) and push the tag.
2. In `contrib/fiber/go.mod`, change the core `require` from the
   `v0.0.0-00010101000000-000000000000` placeholder to that tag (it must contain
   every core API the adapter uses); run `go mod tidy` there and commit.
3. Tag that commit `contrib/fiber/v0.1.0-beta.4` and push the tag.
4. `contrib/gorm` requires both the core and `contrib/fiber` (the latter for its
   tests only), and keeps a `replace` for each. Change both placeholder
   `require`s to the tags from steps 1 and 3, run `go mod tidy` there, commit,
   and tag that commit `contrib/gorm/v0.1.0-beta.4`.

Until the first `contrib/fiber/` tag is published, `go get
github.com/MagicRodri/go-polyadmin/contrib/fiber` cannot resolve the placeholder
core version; publish the tags together.

## Languages

French and Russian are **on by default** next to English: a visitor
whose browser asks for either gets the admin's own text in it, and the
language switcher appears in the header. Their catalogs are **drafts,
awaiting review by native speakers** — corrections are welcome. To keep
an admin English-only, restrict the supported set:

```go
admin := core.New(core.WithLocales("en"), /* ... */)
```

See [`docs/i18n.md`](docs/i18n.md) for the rest: translating your own
strings, the switcher, and how a request's language is chosen.

## Upgrading: CSRF protection

Every mutating route now requires a CSRF token, on by default. The
framework's own pages carry it automatically; a **custom `AdminPage` that
renders its own `<form>`** must add the hidden field, or its posts will be
rejected with `403`:

```html
{{template "csrf-field" .CSRFToken}}
```

Forms that only submit via `hx-post` need no change. Every admin response
also sends `X-Frame-Options: DENY`. See
[`docs/authentication.md`](docs/authentication.md#csrf-protection) for the
cookie/header/field names, the proxy caveat (`X-Forwarded-Proto`), and how
to opt out.

## Status

Feature-complete: CRUD, search/filter/sort/pagination, relations +
autocomplete, record/bulk Actions, a dashboard, CSV and XLSX export,
delete previews (see what a delete takes with it; protected records
block it), an optional login page, an optional audit log,
flash-message toasts,
and per-resource/per-widget template overrides. The API is idiomatic Go
— see the package doc comments in `core/*.go` and `contrib/fiber/*.go` for
specifics (functional options, `BaseModelAdmin` embedding instead of
inheritance, comma-ok lookups, `Disable*` flags, field/form HTML built
in Go functions rather than inside `html/template` files for tighter
control over escaping).

```
.
├── core/       # Admin, ModelAdmin, Field, Relation, Filter, query
│               # pipeline, Authenticator/Authorizer, Dashboard/Widget,
│               # Exporter (CSV, XLSX via excelize)
├── fiber/      # router (Mount), handlers, auth/permission wiring,
│               # relation options, export, html/template rendering
├── templates/  # embedded (go:embed) html/template files
└── static/     # unused by the framework itself -- see WithStaticDir
```

Run the tests:

```bash
go test ./...
```

Run the reference app:

```bash
cd examples/fiber
go run .
# open http://127.0.0.1:3000/admin
```

## Things worth knowing up front

- **XLSX cells are all strings.** `core.CellValue` already stringifies
  everything for CSV, and `XLSXExporter` reuses it rather than
  re-deriving typed cells. XLSX support (via excelize) is a normal
  dependency of `core`, not an opt-in extra.
- **Template overrides require a `{{define "content"}}` block.**
  `html/template` needs named blocks to layer content into `base.html`,
  so an override file must define a `"content"` block — and a custom
  widget's own template must define a block named after its own
  `Template()` value. See [`docs/templates.md`](docs/templates.md).
- **`NavCategory` field, `Category()` method.** A Go struct can't have
  a field and a method share a name, so `BaseModelAdmin`'s sidebar
  grouping lives in a `NavCategory` field backing a `Category()`
  method — the same split as `SlugOverride`/`Slug()`.

## Adapter contract for `ModelAdmin` implementations

`GetQueryset` must return `[]any` (not a concrete `[]T`) for the
adapter's list/detail/relation-option code to work — Go doesn't
implicitly convert between slice types. See `examples/fiber/user_admin.go`
for the pattern.

## Contributing

Issues and pull requests are welcome at
[MagicRodri/go-polyadmin](https://github.com/MagicRodri/go-polyadmin).

go-polyadmin ships alongside the Python package
[polyadmin](https://github.com/MagicRodri/polyadmin), kept at parity. A
change to behaviour, markup or a translatable string usually belongs in
both; if yours only touches one side, say so in the pull request so the
other can follow.

- Run `./scripts/test.sh` (see [Running the tests](#running-the-tests)).
  The browser suite drives the example app in a real browser with
  Playwright, on Python; see [`browsertests/README.md`](browsertests/README.md).
- Add a test with the change, in the module it touches.
- New user-facing text goes through `core.T`/`core.N_` (or the templates'
  `t`) and into `locales/fr.json` and `locales/ru.json`;
  `contrib/fiber`'s catalog test fails on a missing or unused entry.
- Keep `core` free of web-framework and ORM imports; `core/layering_test.go`
  enforces it. Integrations live under `contrib/`, each its own module.
- Note user-facing changes under **Unreleased** in
  [`CHANGELOG.md`](CHANGELOG.md).
- Update [`docs/`](docs/) when behaviour changes. Examples there and in
  [`examples/`](examples/) use neutral sample domains.

## License

[MIT](LICENSE) © 2026
