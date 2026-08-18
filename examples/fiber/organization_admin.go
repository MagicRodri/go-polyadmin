package main

import (
	"context"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
)

type OrganizationAdmin struct {
	core.BaseModelAdmin
	repository *OrganizationRepository
}

func NewOrganizationAdmin(repository *OrganizationRepository) *OrganizationAdmin {
	return &OrganizationAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "Organization",
			SlugOverride:     "organizations",
			NavCategory:      "Directory",
			DisplayFields:    []string{"ID", "Name"},
			FormFieldNames:   []string{"Name"},
			SearchFieldNames: []string{"Name"},
			DeclaredFields:   []core.Field{core.NewField("Name", core.FieldTypeString, core.WithRequired())},
			// Shows each Organization's Users inline on its own
			// create/detail/edit pages -- see docs/inlines.md.
			DeclaredInlines: []core.Inline{core.NewTabularInline("users", "Organization")},
		},
		repository: repository,
	}
}

func (a *OrganizationAdmin) GetQueryset(ctx context.Context) (any, error) {
	orgs := a.repository.List()
	out := make([]any, len(orgs))
	for i, o := range orgs {
		out[i] = o
	}
	return out, nil
}

func (a *OrganizationAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	if org := a.repository.Get(id); org != nil {
		return org, nil
	}
	return nil, nil
}

func (a *OrganizationAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	name, _ := data["Name"].(string)
	return a.repository.Create(name), nil
}
