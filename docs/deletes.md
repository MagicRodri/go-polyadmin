# Delete previews

Before a delete goes through, the admin can say what else it takes with
it — and refuse outright when storage or permissions would not allow it.
**What a delete cascades to is your decision**: the framework does not
own persistence, so it cannot work the cascade out. It only asks.

Nothing changes until a ModelAdmin answers the question.

## What the preview shows

On the delete page, the heading names the record — "Delete
«acme@example.com»?" — whether or not anything else is affected. That
much is always on.

A ModelAdmin that answers adds two things:

- **This will also delete** — one card per related type, headed with the
  type's name and the full count ("User (40)"). Up to ten records are
  listed, linked when the viewer may open them; the rest become "…and 30
  more". Records the viewer may not see are counted as "2 you can't
  view" and never named.
- **This can't be deleted** — a callout that *replaces* the Delete
  button when anything blocks. It says which records must go first, or
  which types the account may not delete.

Two other entry points change with it. A list row's **Delete** becomes a
link to that page instead of deleting in place, because there is now
something to read before confirming. And **delete_selected** grows a
server-rendered confirmation page, described under
[Bulk deletes](#bulk-deletes).

## Implementing it

Implement one method:

```go
type DeletePreviewer interface {
    DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error)
}
```

`objects` is always a list — one record from the delete page, the whole
selection from `delete_selected` — so one implementation serves both,
and a bulk delete costs one query per relation rather than one per row:

```go
func (a *OrganizationAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
    ids := make([]any, len(objects))
    for i, obj := range objects {
        ids[i] = obj.(*Organization).ID
    }
    placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")

    var total int
    row := a.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM users WHERE organization_id IN (`+placeholders+`)`, ids...)
    if err := row.Scan(&total); err != nil {
        return core.DeletePreview{}, err
    }

    rows, err := a.db.QueryContext(ctx,
        `SELECT id, email FROM users WHERE organization_id IN (`+placeholders+`) ORDER BY id LIMIT 10`, ids...)
    if err != nil {
        return core.DeletePreview{}, err
    }
    defer rows.Close()
    var sample []any
    for rows.Next() {
        u := &User{}
        if err := rows.Scan(&u.ID, &u.Email); err != nil {
            return core.DeletePreview{}, err
        }
        sample = append(sample, u)
    }
    return core.DeletePreview{Cascades: []core.DeleteGroup{
        {Resource: "users", Objects: sample, Total: total},
    }}, rows.Err()
}
```

A `core.DeleteGroup` is one related type's share:

| Field | Meaning |
|---|---|
| `Resource` | The slug of a registered ModelAdmin, or `""` for a type the admin does not manage — then `Label` is required. An unknown slug is an error. |
| `Label` | The card's heading, translated at render. Empty falls back to the resource's verbose name. |
| `Objects` | A sample. At most `core.DeletePreviewSample` (10) are listed; pass fewer and nothing is lost. |
| `Total` | The full count. Raised to `len(Objects)` when smaller, so a group you did not count still shows an honest number. |

A group whose total is zero is dropped, so "no users" costs no card.

**The preview describes your delete; it never performs it.** The
cascade still happens in the ModelAdmin's own `Delete`:

```go
func (a *OrganizationAdmin) Delete(ctx context.Context, obj any) error {
    // ... delete the users, then the organization
}
```

If your database does the cascading through `ON DELETE CASCADE`, `Delete`
stays a single statement and the preview simply reports what the
constraint will do.

## Protected records versus permission blocks

A delete is blocked for either of two reasons, and both replace the
Delete button with the refusal callout.

**Protected** groups are records that *storage* would refuse to orphan —
a role still held by users, a row behind `ON DELETE RESTRICT`:

```go
func (a *RoleAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
    holders := a.holders(objects)
    sample := make([]any, len(holders))
    for i, u := range holders {
        sample[i] = u
    }
    return core.DeletePreview{Protected: []core.DeleteGroup{
        {Resource: "users", Objects: sample, Total: len(holders)},
    }}, nil
}
```

Keep the rule in `Delete` as well. The preview is what the admin shows;
storage keeps its own guarantee, as the constraint would:

```go
func (a *RoleAdmin) Delete(ctx context.Context, obj any) error {
    role := obj.(*Role)
    if len(a.holders([]any{role})) > 0 {
        return fmt.Errorf("role %q is still assigned", role.Name)
    }
    return a.repository.Delete(role)
}
```

**Permission blocks** need nothing from you. A cascade into a type the
principal may not delete refuses the whole delete, because otherwise
this delete would be a way around that rule. The check is the same pair
the rest of the admin uses: the target ModelAdmin's own `CanDelete`,
then the Authorizer's `<slug>.delete`. See
[`permissions.md`](permissions.md).

Two consequences worth stating plainly:

- **Types the admin does not manage never block on permission.** There
  is no ModelAdmin to ask, so a `Label`-only group is informational.
- **Records the principal may not view are counted, not named.** The
  count is deliberate: hiding that something exists would make the
  preview lie about the size of the delete.

## Bulk deletes

On a previewing ModelAdmin, `delete_selected` no longer fires a
browser confirm. The first POST answers with a confirmation page — the
selected records, the same preview cards, and a Delete button that posts
the form back with `_confirmed=1`. Nothing is deleted until that second
POST.

What the form carries depends on how the selection was made:

- **Ticked rows** post their `pks` back, so the confirmed set is exactly
  the reviewed one.
- **"Select all N matching"** posts the list's `search`, `sort` and
  `filter[...]` values instead, because a checkbox only reaches the rows
  on screen. Alongside them rides `_fingerprint`, a hash of the primary
  keys the query resolved to when you reviewed it.

If the matching set changed in between — someone added a record, an
import ran — the fingerprint no longer matches, and the page comes back
with "The selection changed since you reviewed it." and deletes nothing.
The new set is right there to review.

The form also carries `_return`: the list you came from, with its search
and filters intact, so confirming lands you back where you were rather
than on the bare list.

This also applies to a `delete_selected` action of your own. The
confirmation is keyed on the action's name, not on the built-in
implementation.

## Enforcement

The preview is not advice. **Every delete route re-runs it immediately
before deleting**, so a stale page, a crafted request or a direct POST
all meet the same refusal:

| Route | When blocked |
|---|---|
| `POST /{slug}/{pk}/delete` | 303 back to the delete page, which says why |
| `DELETE /{slug}/{pk}/delete` (htmx row delete) | `HX-Redirect` to the delete page |
| `DELETE /{slug}/{pk}/inlines/{child}/{childPK}` | 200 with the inline section re-rendered and the refusal above it, so the parent form's unsaved edits survive |
| `POST /{slug}/actions/delete_selected` | The confirmation page again, carrying the refusal |

A preview that returns an error deletes nothing: the error propagates
and the request fails. A ModelAdmin without the capability is never
asked, and its routes behave exactly as before.

## What it doesn't do

- **It does not compute cascades.** Only your code knows the schema.
- **It does not walk the tree.** What you return is what is shown: a
  second level of cascade is yours to include if it matters.
- **It does not write audit entries for cascaded records.** One entry is
  recorded for the record the admin deleted — see
  [`audit.md`](audit.md).
- **It previews deletes only.** Other actions still use their `Confirm`
  text.
