# The list view

Beyond search, filters, sorting and pagination, four options shape how a
list behaves and where it leads. All four are opt-in except the first,
which is on because the alternative — losing a reader's filters the
moment they open a record — is rarely what anyone wants.

## Keeping the reader's place

Open a record from a filtered list, save it, and you land back in that
list, with the search, filters, sort and page you left. This is on by
default:

```go
core.BaseModelAdmin{
    DisablePreserveFilters: true, // off, if you'd rather always land on the bare list
}
```

**How it travels.** The list's links carry one reserved parameter,
`_list`, holding the list's own path and query; the create, edit and
delete forms post it back in a hidden field of the same name. Because it
rides in the page rather than in the browser's history, it survives "Save
and continue editing", a bookmark and a new tab.

Note where it actually shows up. This admin returns you to the
**record's own page** after a save, not to the list, so the token's real
job is to travel with you: the detail page, the edit form and the create
form all carry it, and the **breadcrumb back to the list** is the link
that uses it. Only a delete returns to the list directly.

A token that is not a path under the admin's own base is discarded and
you get the bare list — the same rule `SafeRedirectPath` applies to a
`Referer`.

## Which columns sort

By default every column in `DisplayFields` offers a sort. Restrict it
when a column is computed, or expensive, or simply meaningless to order
by:

```go
core.BaseModelAdmin{
    DisplayFields:      []string{"ID", "Email", "Plan", "LastSeen"},
    SortableFieldNames: []string{"ID", "Email"},
}
```

A column outside the list renders as a plain header, with no sort menu.
The restriction is also enforced server-side: `?sort=Plan` is dropped and
the default ordering applies, so a hand-typed URL cannot reach it either.

`nil` means "every column"; an empty (non-nil) slice means "none".
`OrderingDefault` is exempt — it is the admin's own choice, not user
input, so an admin may sort by a column it does not offer as a header.

## Which cells open the record

The first column links to the record. Name others — or none — with
`LinkFieldNames`:

```go
core.BaseModelAdmin{
    LinkFieldNames: []string{"Email"}, // the address, not the id
}
```

An empty (non-nil) slice links nothing and leaves the row menu as the
only way in. A cell whose value is already a link — a relation, chiefly
— is never wrapped in a second one.

## Filtering by date

A date or datetime field gets the windows a reader actually asks for --
"what came in this week?" -- by declaring a filter, like any other:

```go
core.BaseModelAdmin{
    DeclaredFilters: []core.Filter{core.NewDateFilter("Founded")},
}
```

It renders in the same filter panel as `BooleanFilter` and
`ChoiceFilter`, and offers **Any date**, **Today**, **Past 7 days**,
**This month** and **This year**. The value in the URL is the preset's
own name (`filter[Founded]=7d`), so a filtered list is a link like any
other.

Because it rides in the same `ListRequest` as every other filter,
**exports and `delete_selected` narrow with it**: "all N matching" means
what the panel is showing.

A `ListQuerier` resolves it in its own query. `core.DateFilterRange`
turns a value into the half-open window the in-memory path applies, so
the two cannot drift:

```go
func (a *OrganizationAdmin) ListPage(ctx context.Context, req core.ListRequest) ([]any, int, error) {
    if from, to, ok := core.DateFilterRange(req.Filters["Founded"], time.Now()); ok {
        where = append(where, "founded >= $1 AND founded < $2")
        args = append(args, from, to)
    }
    // ...
}
```

Two details worth knowing: the window is **half-open** (`>= from`,
`< to`), so a row on the boundary belongs to exactly one window; and it
is compared **by date**, so a datetime's clock time never decides whether
it counts as "today". An unrecognised value narrows nothing rather than
failing, so a crafted URL renders the list unfiltered.
