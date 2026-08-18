# PolyAdmin (Go)

Go implementation of PolyAdmin, the cross-language server-rendered
admin framework. See [`docs/`](docs/) for reference documentation.

The Python/FastAPI implementation is a separate repository:
[MagicRodri/polyadmin](https://github.com/MagicRodri/polyadmin). The
two share no runtime code — they're the same design implemented twice,
at feature parity. The `docs/` here cover both, so most pages show
Python and Go side by side.

## Quickstart

```bash
go get github.com/MagicRodri/go-polyadmin
```

No tagged release yet, so `go get` resolves the tip of `main`; pin a
commit if you need reproducibility.

Declare a `ModelAdmin` against your own storage and mount it on a
Fiber app:

```go
package main

import (
	"context"
	"log"
	"strconv"

	"polyadmin/core"
	fiberadapter "polyadmin/fiber"

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
actions, a dashboard, exports), see
[`docs/model-admin.md`](docs/model-admin.md) and the rest of
[`docs/`](docs/).

## Status

Feature-complete alongside the Python/FastAPI adapter — CRUD, search/
filter/sort/pagination, relations + autocomplete, record/bulk Actions,
a dashboard, CSV and XLSX export, flash-message toasts, and
per-resource/per-widget template overrides all exist in both
languages today. The API is idiomatic Go rather than a literal port of
the Python one — see the package doc comments in `core/*.go` and
`fiber/*.go` for specifics (functional options, `BaseModelAdmin`
embedding instead of inheritance, comma-ok lookups, `Disable*` flags
instead of `can_*`, field/form HTML built in Go functions rather than
inside `html/template` files for tighter control over escaping).

```
go/polyadmin/
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
cd go/polyadmin
go test ./...
```

Run the reference app:

```bash
cd examples/fiber
go run .
# open http://127.0.0.1:3000/admin
```

## Differences from the Python/FastAPI adapter

Not gaps — deliberate, idiomatic-Go choices where the two languages'
tooling or type systems don't map onto each other 1:1:

- **XLSX cells are all strings.** `core.CellValue` already stringifies
  everything for CSV; `XLSXExporter` reuses it rather than re-deriving
  typed (int/float/bool) cells the way Python's `openpyxl`-backed
  exporter does. XLSX is also a normal (non-optional) dependency of
  `core` here — Go doesn't have Python's notion of an installable
  extra (`polyadmin[export-xlsx]`).
- **Template overrides require a `{{define "content"}}` block.**
  Python's Jinja templates use `{% extends %}` inheritance, so an
  override can be any standalone template. Go's `html/template` needs
  named blocks to layer content into `base.html`, so an override file
  must define a `"content"` block (and a custom widget's own template
  must define a block named after its own `Template()` value) — see
  [`docs/templates.md`](docs/templates.md).
- **`NavCategory` field, `Category()` method.** A Go struct can't have
  a field and a method share a name, so `BaseModelAdmin`'s sidebar
  grouping lives in a `NavCategory` field backing a `Category()`
  method — the same split as `SlugOverride`/`Slug()`. Python's
  `ModelAdmin.category` is just one attribute.

## Adapter contract for `ModelAdmin` implementations

`GetQueryset` must return `[]any` (not a concrete `[]T`) for the Fiber
adapter's list/detail/relation-option code to work — Go doesn't
implicitly convert between slice types. See `examples/fiber/user_admin.go`
for the pattern.
