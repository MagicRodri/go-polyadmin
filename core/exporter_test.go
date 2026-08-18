package core

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func exportCSV(t *testing.T, admin *Admin, modelAdmin ModelAdmin, objects []any, columns []string) string {
	t.Helper()
	var buf bytes.Buffer
	writer := NewCSVRowWriter(csv.NewWriter(&buf))
	if err := (CSVExporter{}).Write(writer, admin, modelAdmin, objects, columns); err != nil {
		t.Fatalf("write: %v", err)
	}
	return buf.String()
}

func TestCSVExporterHeaderAndRows(t *testing.T) {
	userAdmin := newInMemoryUserAdmin()
	mustCreateUser(t, userAdmin, "john@example.com", true)
	mustCreateUser(t, userAdmin, "mary@example.com", true)
	admin := New(WithModelAdmins(userAdmin))

	csvText := exportCSV(t, admin, userAdmin, usersQueryset(t, userAdmin), userAdmin.ListDisplay())
	lines := strings.Split(strings.TrimSpace(csvText), "\n")
	if lines[0] != "Id,Email,Is Active" {
		t.Fatalf("got header %q", lines[0])
	}
	if !strings.Contains(lines[1], "john@example.com") || !strings.Contains(lines[2], "mary@example.com") {
		t.Fatalf("got %v", lines)
	}
}

func TestCSVExporterRespectsColumnSubset(t *testing.T) {
	userAdmin := newInMemoryUserAdmin()
	mustCreateUser(t, userAdmin, "john@example.com", true)
	admin := New(WithModelAdmins(userAdmin))

	csvText := exportCSV(t, admin, userAdmin, usersQueryset(t, userAdmin), []string{"Email"})
	lines := strings.Split(strings.TrimSpace(csvText), "\n")
	if lines[0] != "Email" || lines[1] != "john@example.com" {
		t.Fatalf("got %v", lines)
	}
}

func TestCellValueResolvesForeignKeyToDisplayLabel(t *testing.T) {
	orgAdmin := orgAdminFixture{BaseModelAdmin{ModelName: "Organization", SlugOverride: "organizations", DeclaredFields: []Field{NewField("Name", FieldTypeString)}}}
	admin := New(WithModelAdmins(orgAdmin))
	relation := Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}
	field := NewField("Organization", FieldTypeForeignKey, WithRelation(relation))

	org := &Organization{ID: 1, Name: "Acme"}
	if got := CellValue(admin, field, &Item{Organization: org}); got != "Acme" {
		t.Fatalf("got %q", got)
	}
}

func TestCellValueForeignKeyNilIsEmptyString(t *testing.T) {
	orgAdmin := orgAdminFixture{BaseModelAdmin{ModelName: "Organization", SlugOverride: "organizations", DeclaredFields: []Field{NewField("Name", FieldTypeString)}}}
	admin := New(WithModelAdmins(orgAdmin))
	relation := Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}
	field := NewField("Organization", FieldTypeForeignKey, WithRelation(relation))

	if got := CellValue(admin, field, &Item{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

// orgAdminFixture just embeds BaseModelAdmin so it satisfies ModelAdmin
// for the CellValue tests above, which only need Field()/GetPK().
type orgAdminFixture struct {
	BaseModelAdmin
}

func TestCSVExporterStreamsThroughBufio(t *testing.T) {
	userAdmin := newInMemoryUserAdmin()
	for i := 0; i < 500; i++ {
		mustCreateUser(t, userAdmin, "user@example.com", true)
	}
	admin := New(WithModelAdmins(userAdmin))

	csvText := exportCSV(t, admin, userAdmin, usersQueryset(t, userAdmin), userAdmin.ListDisplay())
	lines := strings.Split(strings.TrimSpace(csvText), "\n")
	if len(lines) != 501 { // header + 500 rows
		t.Fatalf("got %d lines", len(lines))
	}
}
