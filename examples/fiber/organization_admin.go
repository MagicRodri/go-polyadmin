package main

import (
	"context"
	"strconv"
	"time"

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
			DisplayFields:    []string{"ID", "Name", "Founded", "Balance"},
			FormFieldNames:   []string{"Name", "Founded", "Balance"},
			SearchFieldNames: []string{"Name"},
			DeclaredFields: []core.Field{
				core.NewField("Name", core.FieldTypeString, core.WithRequired()),
				core.NewField("Founded", core.FieldTypeDate),
				core.NewField("Balance", core.FieldTypeDecimal),
			},
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
	founded, balance := parseOrganizationFormFields(data)
	return a.repository.Create(name, founded, balance), nil
}

func (a *OrganizationAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	org, _ := obj.(*Organization)
	if org == nil {
		return nil, nil
	}
	name, _ := data["Name"].(string)
	founded, balance := parseOrganizationFormFields(data)
	return a.repository.Update(org, name, founded, balance), nil
}

// parseOrganizationFormFields parses the two fields Task 11 added onto
// Organization. A date input posts YYYY-MM-DD; an unparseable or absent
// value falls back to the zero time / zero balance rather than failing
// the whole submission, matching Founded and Balance not being marked
// required.
func parseOrganizationFormFields(data map[string]any) (time.Time, float64) {
	var founded time.Time
	if s, _ := data["Founded"].(string); s != "" {
		if parsed, err := time.Parse("2006-01-02", s); err == nil {
			founded = parsed
		}
	}
	var balance float64
	if s, _ := data["Balance"].(string); s != "" {
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			balance = parsed
		}
	}
	return founded, balance
}
