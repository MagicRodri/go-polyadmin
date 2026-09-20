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

Below the presets the panel offers **Custom range**, two date inputs
whose value rides in the same parameter:

```
?filter[Founded]=7d                       a preset
?filter[Founded]=2026-01-01:2026-03-01    a range
?filter[Founded]=2026-01-01:              from that date through today
```

The range is **inclusive of both endpoints** — that is what "from X to
Y" means — and `core.DateFilterRange` converts it to the same half-open
window the presets produce, so a `ListQuerier` host resolving the raw
value gets identical results either way. An empty end means "through
today", resolved by the parser rather than by the panel, so the range
still works with scripting off. An empty start, an unparseable date, or
an end before its start narrows nothing, the same rule an unrecognised
preset follows.

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

## Filtering on whether a field is set at all

`EmptyFilter` splits a list on presence, which is the question behind
most "why is this record wrong?" hunts:

```go
core.BaseModelAdmin{
    DeclaredFilters: []core.Filter{core.NewEmptyFilter("Organization")},
}
```

It offers **All**, **Empty** and **Not empty**, and the URL carries
`filter[Organization]=empty` or `=notempty`.

**Empty means unset or blank, never merely zero.** `nil`, `""`, an empty
collection, a nil pointer and a zero time are empty; `0`, `false` and
`"0"` are values somebody chose. A plain `int` or `bool` field can
therefore never be empty — those types cannot represent "unset", so a
nullable number is a `*int`, as it already must be for anything
optional. A `many` relation is empty when it has no members.

A `ListQuerier` host reads the raw value and compares it against the
constants, so the two paths cannot drift:

```go
switch req.Filters["Organization"] {
case core.EmptyFilterEmpty:
    where = append(where, "organization_id IS NULL")
case core.EmptyFilterNotEmpty:
    where = append(where, "organization_id IS NOT NULL")
}
```

## Filtering by a related record

```go
core.BaseModelAdmin{
    DeclaredFilters: []core.Filter{core.NewRelationFilter("Organization")},
}
```

The URL carries the target's primary key — `filter[Organization]=3` —
and a `many` relation matches when any member does, so "users whose
Teams include Platform" works.

**The control follows `AutocompleteFields`.** A relation named there
renders as the same lookup-backed combobox the form uses, pointed at the
target's own `/lookup` route; every other relation renders as a list of
links over the target's whole queryset, uncapped, exactly as the form's
non-autocomplete `<select>` already does. Declaring the relation in
`AutocompleteFieldNames` is the answer to a large target, and it is the
same answer in both places.

A reader who may not view the target resource does not get the filter at
all — it is dropped from the panel rather than shown empty.

**One limitation worth knowing.** `Apply` receives the parent
ModelAdmin, not the registry, so it cannot call the target's own
`GetPK`. It uses the same default lookup `BaseModelAdmin.GetPK` does. If
the target declares a custom `PK` func, set the filter's `RelatedPK` to
match:

```go
filter := core.NewRelationFilter("Organization")
filter.RelatedPK = func(related any) any { return related.(*Org).Code }
```

## Writing your own filter

A filter is an interface, not a closed set. Implement four methods and
declare it like any built-in; Django calls this a `SimpleListFilter`,
and the two halves are the same: the options, and the constraint.

```go
type planFilter struct{}

func (planFilter) Name() string  { return "Plan" }
func (planFilter) Label() string { return "Plan" }

func (planFilter) ChoicesWithLabels() [][2]string {
    return [][2]string{{"", "All"}, {"paid", "Paid"}, {"free", "Free"}}
}

func (planFilter) Apply(objects []any, raw string, modelAdmin core.ModelAdmin) []any {
    if raw == "" {
        return objects // "" always means "no filter"
    }
    field, ok := modelAdmin.Field("Plan")
    if !ok {
        return objects
    }
    out := make([]any, 0, len(objects))
    for _, obj := range objects {
        plan, _ := field.GetValue(obj).(string)
        if (plan != "Free") == (raw == "paid") {
            out = append(out, obj)
        }
    }
    return out
}
```

```go
core.BaseModelAdmin{
    DeclaredFilters: []core.Filter{planFilter{}},
}
```

Three rules make a host filter equal to a built-in rather than
cosmetic:

- **`""` always means "no filter".** It is the first choice, it is what
  Clear all sets, and `Apply` must return its input unchanged for it.
- **An unrecognised value narrows nothing.** The value comes from a URL,
  so treat anything you do not recognise as absent rather than failing —
  a crafted link should render the list, not an error.
- **`Apply` gets the raw string.** Parsing is the filter's own job,
  because only it knows what its values mean.

Because it rides in the same `ListRequest` as every built-in, **exports
and `delete_selected` narrow with it**: "all N matching" means what the
panel is showing. A `ListQuerier` host reads the same raw value out of
`req.Filters` and resolves it in its own query.

A filter that needs a control other than a list of links declares one by
implementing `core.FilterControl` — that is how `DateFilter` gets its
range inputs and `RelationFilter` its combobox. A filter that does not
implement it is a choice list, which is what almost every filter wants.
