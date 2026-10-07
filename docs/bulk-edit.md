# Bulk editing

A ModelAdmin that sets `BulkEditFieldNames` gets an **Edit selected** action
on its list page. It changes the same fields on every selected record in one
go:

```go
core.BaseModelAdmin{
	FormFieldNames:     []string{"Email", "IsActive", "Plan", "Organization"},
	BulkEditFieldNames: []string{"IsActive", "Plan", "Organization"},
}
```

Each name must be one of the form fields and not read-only; a wrong name makes
`Register` panic. The action is offered only when `CanUpdate()` is true, and
it requires the `{slug}.update` permission on top of `.view`. It is a
list-only action: it never appears on a detail page, even when named in
`DeclaredDetailActions`.

A ModelAdmin written without `core.BaseModelAdmin` opts in by implementing
`core.BulkEditor` (`BulkEditFields() []string`) and appending
`core.NewBulkEditAction(fields)` to its own `Actions()`.

## The form

Picking **Edit selected** opens a form with one row per field, using the
field's own widget — the same select, switch or relation picker the edit form
shows, including `AutocompleteFieldNames`. Each row has a **Change**
checkbox, and the field stays disabled until it is ticked. Only ticked fields
are parsed, validated and applied; a row left unticked is ignored even when a
value is posted for it. Submitting with nothing ticked redisplays the form
with "Choose at least one field to change."

## `Update` receives only the ticked fields

The default implementation calls your ModelAdmin's own
`Update(ctx, obj, data)` once per record, with `data` holding **only the
ticked fields** — a PATCH, not the full form the edit page posts. An `Update`
that reads every key and writes what it finds would clear the fields nobody
touched:

```go
plan, _ := data["Plan"].(string)   // "" when Plan was not ticked
user.Plan = plan                   // wipes it

if plan, ok := data["Plan"].(string); ok {
	user.Plan = plan               // keeps it
}
```

Write `Update` so that a missing key leaves the value as it is. The edit form
already omits read-only fields, so an `Update` that handles that is most of
the way there.

## One call instead of N

Implement `core.BulkUpdater` to apply the change in a single call, for
example to a backend's bulk endpoint:

```go
func (a *EmployeeAdmin) BulkUpdate(ctx context.Context, objects []any, data map[string]any, p *core.Principal) (string, error) {
	ids := make([]int, len(objects))
	for i, obj := range objects {
		ids[i] = obj.(*Employee).ID
	}
	if err := a.client.PatchEmployees(ctx, ids, data); err != nil {
		return "", err
	}
	return fmt.Sprintf("Updated %d employee(s).", len(objects)), nil
}
```

The returned string is the success message, as for any action.

## All or nothing

Before anything is applied, every selected record is checked: the
authorizer must allow `{slug}.update` on that record, and none of the ticked
fields may be read-only for it (`ReadOnlyFields(obj)`). If any record fails
either check, nothing is changed and the form lists the records that blocked
it.

The default loop stops at the first record whose `Update` returns an error
and reports how far it got — "Updated 3 of 10, then failed: …", as an error
notification — the same way the bulk delete does. Records before the failure
keep their change and are audited as updates; there is no transaction to roll
back. A `*core.RecordFormError` from `Update` contributes its messages after
"then failed:". Implement `BulkUpdater` when your storage can do
better.

## Audit log

Each record gets an ordinary `update` entry, the same as saving it from its
edit page (see [`audit`](audit.md)).

## Replacing the built-in

Declare an action named `core.BulkEditName` in `DeclaredActions` to replace
the built-in, for example with a different label:

```go
a := core.NewBulkEditAction(fields)
a.Label = "Change plan"
DeclaredActions: []core.Action{a},
```
