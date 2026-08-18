package main

import (
	"context"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
)

var organizationRelation = core.Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}

type UserAdmin struct {
	core.BaseModelAdmin
	repository    *UserRepository
	organizations *OrganizationRepository
}

func setActive(ctx context.Context, modelAdmin core.ModelAdmin, objects []any, principal *core.Principal, active bool) (string, error) {
	for _, obj := range objects {
		obj.(*User).IsActive = active
	}
	verb := "Activated"
	if !active {
		verb = "Deactivated"
	}
	return verb + " " + strconv.Itoa(len(objects)) + " user(s).", nil
}

func NewUserAdmin(repository *UserRepository, organizations *OrganizationRepository) *UserAdmin {
	return &UserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName: "User",
			// Shares a sidebar accordion with OrganizationAdmin -- see
			// docs/routing.md's "Sidebar categories" section.
			NavCategory:      "Directory",
			DisplayFields:    []string{"ID", "Email", "IsActive", "Organization"},
			DetailFieldNames: []string{"ID", "Email", "IsActive", "Organization"},
			FormFieldNames:   []string{"Email", "IsActive", "Organization"},
			SearchFieldNames: []string{"Email"},
			DeclaredFilters:  []core.Filter{core.NewBooleanFilter("IsActive")},
			// Routes the "Organization" relation through the /lookup
			// endpoint instead of a same-page <select>
			// populated from every organization -- demonstrates the
			// command widget for a relation that, in a real deployment,
			// could be too large to dump wholesale.
			AutocompleteFieldNames: []string{"Organization"},
			DeclaredActions: []core.Action{
				core.NewAction("activate", func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (string, error) {
					return setActive(ctx, ma, objects, p, true)
				}, core.WithActionLabel("Activate")),
				core.NewAction("deactivate", func(ctx context.Context, ma core.ModelAdmin, objects []any, p *core.Principal) (string, error) {
					return setActive(ctx, ma, objects, p, false)
				}, core.WithActionLabel("Deactivate"), core.WithActionConfirm("Deactivate the selected users?")),
			},
			DeclaredFields: []core.Field{
				core.NewField("Email", core.FieldTypeEmail, core.WithRequired()),
				core.NewField("IsActive", core.FieldTypeBoolean, core.WithDefault(true)),
				core.NewField("Organization", core.FieldTypeForeignKey, core.WithRelation(organizationRelation)),
			},
		},
		repository:    repository,
		organizations: organizations,
	}
}

func (a *UserAdmin) GetQueryset(ctx context.Context) (any, error) {
	users := a.repository.List()
	out := make([]any, len(users))
	for i, u := range users {
		out[i] = u
	}
	return out, nil
}

func (a *UserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	if u := a.repository.Get(id); u != nil {
		return u, nil
	}
	return nil, nil
}

func (a *UserAdmin) resolveOrganization(data map[string]any) *Organization {
	raw, _ := data["Organization"].(string)
	if raw == "" {
		return nil
	}
	id, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return a.organizations.Get(id)
}

func (a *UserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	email, _ := data["Email"].(string)
	isActive, _ := data["IsActive"].(bool)
	return a.repository.Create(email, isActive, a.resolveOrganization(data)), nil
}

func (a *UserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	user := obj.(*User)
	email, _ := data["Email"].(string)
	isActive, _ := data["IsActive"].(bool)
	return a.repository.Update(user, email, isActive, a.resolveOrganization(data)), nil
}

func (a *UserAdmin) Delete(ctx context.Context, obj any) error {
	a.repository.Delete(obj.(*User))
	return nil
}
