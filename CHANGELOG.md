# Changelog

## [Unreleased]

## [0.1.0-beta.4] — 2026-10-08

### Added

- `contrib/gorm`, a new module: `polygorm.New[T](db, base, opts...)` serves a
  GORM model.
  - Search, filters (Boolean, Choice, Empty, Date, Relation), ordering and
    paging run in SQL.
  - Fields are derived from the model.
  - Writes go through GORM, so hooks fire.
  - `WithDefaultFilters` applies until the user picks that filter.
  - Unique, foreign-key and check violations become translated form errors.
- `core.RecordFormError`: a `ModelAdmin` can refuse a save with messages
  shown on the form (status 422), or a delete with the reason shown on the
  delete page. `core.RecordErrorText` gives the text of any write error.
- `core.ActionError`: an action fails with a notification at the level it
  chooses (`"error"` or `"warning"`), and the records listed in `Done` are
  audited. `Err` keeps the cause for `errors.Is` and `errors.As`.
- `core.NewChoicePairsFilter` takes `(value, label)` pairs.
  `ChoiceFilter.Values()` lists the values.
- `core.DefaultFilterer`: an optional capability. The filter panel spells
  "All" out (`filter[name]=`) for a defaulted filter, and list, sort and
  export links keep it.
- Form-wide errors (the `""` key) are shown at the top of the form.

### Changed

- **Breaking:** the Fiber adapter is its own module at
  `github.com/MagicRodri/go-polyadmin/contrib/fiber`; it was
  `github.com/MagicRodri/go-polyadmin/fiber`. The package is still named
  `fiber`.
- **Breaking:** `ChoiceFilter.Choices` is a `[][2]string` of `(value, label)`
  pairs. `NewChoiceFilter(name, []string)` is unchanged.
- **Breaking:** `Renderer.RenderDelete` takes the page's flash messages.
- The built-in bulk delete and bulk edit stop at the first failure with an
  `ActionError` ("Deleted 3 of 10, then failed: …"), so the user gets a
  notification instead of a 500 and the records already handled are audited.

### Fixed

- Form errors now appear in the browser. htmx 1.x does not swap a 422
  response, so a form re-rendered with validation errors left the page
  unchanged.
- The delete page shows notifications, including why a delete was refused.

### Documentation

- `docs/gorm.md`.
- README sections on running the tests, releasing contrib modules,
  contributing and the license.
- Examples in the docs use neutral sample domains.
- The example app gains GORM-backed Clients and Projects under a
  **Projects** sidebar group.

## [0.1.0-beta.3] — 2026-09-30

### Added

- Actions that ask for input: a form page before the action runs.
- Actions can answer with a file (`Download`).
- Bulk edit (`BulkEditFieldNames`): change selected fields on many records.
- Dashboard filters (`NewDateRangeFilter`, `NewSelectFilter`, optionally
  searchable) carried in the URL.
- Lazy, filter-aware widgets loaded from their own route, including donuts
  and tabs.
- `MetricGroup` and `DataTable` widgets. `DataTable` pages, searches and
  colours cells with `Tones`.
- Dashboard exports.
- Widgets placed above the filter bar.

## [0.1.0-beta.2] — 2026-09-24

### Added

- Favicon support.

### Changed

- Image fields render and upload more reliably.

## [0.1.0-beta.1] — 2026-09-23

First beta:

- `ModelAdmin` CRUD with search, filters (including relation, empty and
  date-range filters), sorting and pagination.
- A widget dashboard.
- Relations with lookup comboboxes.
- Inlines.
- Actions placed on the list or the detail page.
- Delete previews.
- CSV and XLSX export.
- Authentication and permissions hooks.
- CSRF protection.
- French and Russian translations.
- The Fiber adapter.

[Unreleased]: https://github.com/MagicRodri/go-polyadmin/compare/v0.1.0-beta.4...HEAD
[0.1.0-beta.4]: https://github.com/MagicRodri/go-polyadmin/compare/v0.1.0-beta.3...v0.1.0-beta.4
[0.1.0-beta.3]: https://github.com/MagicRodri/go-polyadmin/compare/v0.1.0-beta.2...v0.1.0-beta.3
[0.1.0-beta.2]: https://github.com/MagicRodri/go-polyadmin/compare/v0.1.0-beta.1...v0.1.0-beta.2
[0.1.0-beta.1]: https://github.com/MagicRodri/go-polyadmin/releases/tag/v0.1.0-beta.1
