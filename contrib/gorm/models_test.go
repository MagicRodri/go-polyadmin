package gorm

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
	"github.com/glebarez/sqlite"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Author struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"size:100;uniqueIndex;not null"`
	Active   *bool
	Born     *time.Time `gorm:"type:date"`
	Kind     string     `gorm:"size:20;default:fiction"`
	Nickname *string
	Created  *time.Time
	Books    []Book `gorm:"constraint:OnDelete:RESTRICT"`
	Tags     []Tag  `gorm:"many2many:author_tags"`
}

type Book struct {
	ID        uint   `gorm:"primaryKey"`
	Title     string `gorm:"not null"`
	Pages     int    `gorm:"not null"`
	Published bool   `gorm:"default:true"`
	AuthorID  *uint
	Author    *Author
}

type Tag struct {
	ID    uint   `gorm:"primaryKey"`
	Label string `gorm:"uniqueIndex"`
}

type Gadget struct {
	ID   string `gorm:"primaryKey"`
	Name string `gorm:"not null"`
	Slug string
}

func (g *Gadget) BeforeCreate(*gormdb.DB) error {
	if g.ID == "" {
		g.ID = "g-" + g.Name
	}
	return nil
}

func (g *Gadget) BeforeSave(*gormdb.DB) error {
	g.Slug = strings.ToLower(g.Name)
	return nil
}

type Employee struct {
	ID        uint `gorm:"primaryKey"`
	Name      string
	ManagerID *uint
	Reports   []Employee `gorm:"foreignKey:ManagerID"`
}

type CompositeKey struct {
	A int `gorm:"primaryKey"`
	B int `gorm:"primaryKey"`
}

var hookLog []string

func (b *Book) AfterCreate(*gormdb.DB) error { hookLog = append(hookLog, "after_create"); return nil }
func (b *Book) AfterUpdate(*gormdb.DB) error { hookLog = append(hookLog, "after_update"); return nil }
func (b *Book) AfterDelete(*gormdb.DB) error { hookLog = append(hookLog, "after_delete"); return nil }

func openDB(t *testing.T) *gormdb.DB {
	t.Helper()
	return openDBWith(t, &gormdb.Config{})
}

func openDBWith(t *testing.T, config *gormdb.Config) *gormdb.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_pragma=foreign_keys(1)"
	config.Logger = logger.Default.LogMode(logger.Silent)
	db, err := gormdb.Open(sqlite.Open(dsn), config)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Author{}, &Tag{}, &Book{}, &Gadget{}, &Employee{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func mustCreate(t *testing.T, db *gormdb.DB, values ...any) {
	t.Helper()
	for _, v := range values {
		if err := db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
}

var authorRelation = core.Relation{Name: "Author", Target: "authors", DisplayField: "Name"}

func authorAdmin(t *testing.T, db *gormdb.DB, opts ...Option) *ModelAdmin[Author] {
	t.Helper()
	admin, err := New[Author](db, core.BaseModelAdmin{
		ModelName:        "Author",
		DisplayFields:    []string{"ID", "Name", "Active", "Kind", "Born"},
		FormFieldNames:   []string{"Name", "Active", "Kind", "Born", "Nickname", "Created"},
		SearchFieldNames: []string{"Name", "Nickname"},
		DeclaredFilters: []core.Filter{
			core.NewBooleanFilter("Active"),
			core.NewChoicePairsFilter("Kind", [][2]string{{"fiction", "Fiction"}, {"science", "Science"}}),
			core.NewEmptyFilter("Nickname"),
			core.NewDateFilter("Born"),
			core.NewEmptyFilter("Books"),
			core.NewRelationFilter("Tags"),
		},
	}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return admin
}

func bookAdmin(t *testing.T, db *gormdb.DB) *ModelAdmin[Book] {
	t.Helper()
	admin, err := New[Book](db, core.BaseModelAdmin{
		ModelName:        "Book",
		DisplayFields:    []string{"ID", "Title", "Author", "Pages"},
		FormFieldNames:   []string{"Title", "Author", "Pages", "Published"},
		SearchFieldNames: []string{"Title"},
		DeclaredFields:   []core.Field{core.NewField("Author", core.FieldTypeForeignKey, core.WithRelation(authorRelation))},
		DeclaredFilters: []core.Filter{
			core.NewRelationFilter("Author"),
			core.NewChoiceFilter("Pages", []string{"100", "200"}),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return admin
}
