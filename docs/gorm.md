# GORM

`go get github.com/MagicRodri/go-polyadmin/contrib/gorm`

`polygorm.New` serves a GORM model: one generic call, no data-access hooks of
your own.

```go
import (
	"github.com/MagicRodri/go-polyadmin/core"
	polygorm "github.com/MagicRodri/go-polyadmin/contrib/gorm"
)

books, err := polygorm.New[Book](db, core.BaseModelAdmin{
	ModelName:        "Book",
	DisplayFields:    []string{"ID", "Title", "Author", "IsArchived"},
	FormFieldNames:   []string{"Title", "Author", "IsArchived"},
	SearchFieldNames: []string{"Title"},
	DeclaredFields: []core.Field{
		core.NewField("Author", core.FieldTypeForeignKey,
			core.WithRelation(core.Relation{Name: "Author", Target: "authors", DisplayField: "Name"})),
	},
	DeclaredFilters: []core.Filter{core.NewBooleanFilter("IsArchived"), core.NewRelationFilter("Author")},
}, polygorm.WithDefaultFilters(map[string]any{"IsArchived": false}))
if err != nil {
	log.Fatal(err)
}
admin := core.New(core.WithModelAdmins(books))
```

- Names are Go struct field names; columns come from GORM's schema and
  naming strategy.
- Search, filters, ordering and paging run in SQL (`ListPage`). Search is
  case-insensitive and matches `%` and `_` literally.
- `WithDefaultFilters` applies until the user picks that filter, "All"
  included; each key must name a column. The filter panel spells "All" out
  as `filter[name]=`.
- Supported filters: `BooleanFilter`, `ChoiceFilter`, `EmptyFilter`,
  `DateFilter`, `RelationFilter`. `New` returns an error for any other, for
  a filter or default naming nothing, for a composite primary key, and for a
  form field that is a relationship other than belongs-to.
- Fields you don't declare are derived from the Go type (`bool`, integers,
  floats, `string`, `time.Time` — a `type:date` tag makes it a date). A
  NOT NULL column with no default is required. Relations are declared, as
  above.
- A belongs-to field writes its foreign key. Has-one, has-many and
  many-to-many fields are read-only; deleting a record removes its
  many-to-many links.
- Writes go through GORM (`Create` and `Delete` each in a transaction,
  `Updates` of the whole row), so the model's hooks fire for every create, edit,
  bulk edit and delete and what they set is saved, and a submitted `false`
  or `0` is written even over a column default.
- A unique, foreign-key or check violation becomes a form error
  (`RecordFormError`) with a translated message rather than a 500; a
  refused delete sends the user back to the delete page with the reason.
  The dialect should implement `gorm.ErrorTranslator` (the official drivers
  and `glebarez/sqlite` do).
- A primary key that doesn't convert, or that the database rejects, reads
  as "not found".
