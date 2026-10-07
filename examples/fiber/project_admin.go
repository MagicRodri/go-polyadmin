package main

import (
	"github.com/MagicRodri/go-polyadmin/core"
	polygorm "github.com/MagicRodri/go-polyadmin/contrib/gorm"
	"gorm.io/gorm"
)

var clientRelation = core.Relation{Name: "Client", Target: "clients", DisplayField: "Name"}

var projectStatuses = [][2]string{{"planned", "Planned"}, {"active", "Active"}, {"done", "Done"}}

// newProjectAdmins serves Client and Project straight from SQLite through
// contrib/gorm: no repository, no hooks of its own.
func newProjectAdmins(db *gorm.DB) (*polygorm.ModelAdmin[Client], *polygorm.ModelAdmin[Project], error) {
	clients, err := polygorm.New[Client](db, core.BaseModelAdmin{
		ModelName:        "Client",
		NavCategory:      "Projects",
		DisplayFields:    []string{"ID", "Name"},
		FormFieldNames:   []string{"Name"},
		SearchFieldNames: []string{"Name"},
	})
	if err != nil {
		return nil, nil, err
	}
	statuses := make([]any, len(projectStatuses))
	for i, s := range projectStatuses {
		statuses[i] = s[0]
	}
	projects, err := polygorm.New[Project](db, core.BaseModelAdmin{
		ModelName:              "Project",
		NavCategory:            "Projects",
		DisplayFields:          []string{"ID", "Name", "Client", "Status", "Active", "Due"},
		FormFieldNames:         []string{"Name", "Client", "Status", "Active", "Due"},
		SearchFieldNames:       []string{"Name"},
		AutocompleteFieldNames: []string{"Client"},
		DeclaredFields: []core.Field{
			core.NewField("Client", core.FieldTypeForeignKey, core.WithRelation(clientRelation)),
			core.NewField("Status", core.FieldTypeEnum, core.WithChoices(statuses...), core.WithRequired()),
		},
		DeclaredFilters: []core.Filter{
			core.NewChoicePairsFilter("Status", projectStatuses),
			core.NewBooleanFilter("Active"),
			core.NewRelationFilter("Client"),
			core.NewDateFilter("Due"),
		},
	}, polygorm.WithDefaultFilters(map[string]any{"Active": true}))
	if err != nil {
		return nil, nil, err
	}
	return clients, projects, nil
}
