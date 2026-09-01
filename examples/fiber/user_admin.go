package main

import (
	"context"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
)

var organizationRelation = core.Relation{Name: "Organization", Target: "organizations", DisplayField: "Name"}

// Cardinality many is what marks this as the collection side; the
// adapter reads the field's current value as []any and renders every
// role in the target's queryset as a choice.
var rolesRelation = core.Relation{Name: "Roles", Target: "roles", DisplayField: "Name", Cardinality: core.CardinalityMany}

type UserAdmin struct {
	core.BaseModelAdmin
	repository    *UserRepository
	organizations *OrganizationRepository
	roles         *RoleRepository
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

func NewUserAdmin(repository *UserRepository, organizations *OrganizationRepository, roles *RoleRepository) *UserAdmin {
	return &UserAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName: "User",
			// Shares a sidebar accordion with OrganizationAdmin -- see
			// docs/routing.md's "Sidebar categories" section.
			NavCategory:      "Directory",
			DisplayFields:    []string{"ID", "Email", "IsActive", "Plan", "Organization"},
			DetailFieldNames: []string{"ID", "Email", "IsActive", "Plan", "Organization", "Roles"},
			// Roles is on the form but not in DisplayFields: a
			// many-to-many column costs a lookup per row and reads as
			// noise in a table, which is why Django keeps it off
			// list_display too.
			// Grouped rather than flat, to exercise fieldsets -- the
			// other admins in this app stay flat, so both paths have
			// example coverage. Declaring these replaces FormFieldNames:
			// the groups are the form's field list.
			DeclaredFieldsets: []core.Fieldset{
				{Fields: []string{"Email", "IsActive"}},
				{Title: "Membership", Description: "Where this user belongs and what they may do.",
					Fields: []string{"Plan", "Organization", "Roles"}},
			},
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
				// HelpText is where an ORM/DB column comment lands. It
				// shows under the control on the form and under the
				// label on the detail page.
				core.NewField("Email", core.FieldTypeEmail, core.WithRequired(),
					core.WithHelpText("Used to sign in, and the address notifications go to.")),
				core.NewField("IsActive", core.FieldTypeBoolean, core.WithDefault(true),
					core.WithHelpText("Inactive users keep their data but cannot sign in.")),
				// Enum + choices renders as ui/select: a hidden input
				// carries the value, so it posts like a native <select>.
				core.NewField("Plan", core.FieldTypeEnum,
					core.WithChoices("Free", "Pro", "Enterprise"), core.WithDefault("Free"),
					core.WithHelpText("Determines feature limits and billing tier.")),
				core.NewField("Organization", core.FieldTypeForeignKey, core.WithRelation(organizationRelation)),
				// Renders as the searchable multi-select
				// (ui/multi-select.html) -- the whole point of seeding
				// eight roles below.
				core.NewField("Roles", core.FieldTypeManyToMany, core.WithRelation(rolesRelation)),
			},
		},
		repository:    repository,
		organizations: organizations,
		roles:         roles,
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

// resolveRoles turns the posted pks back into role objects. The form
// posts one value per selection under "Roles" (exactly what a
// <select multiple> posted), which parseFormData hands over as
// []string.
func (a *UserAdmin) resolveRoles(data map[string]any) []any {
	raw, _ := data["Roles"].([]string)
	out := make([]any, 0, len(raw))
	for _, value := range raw {
		id, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if role := a.roles.Get(id); role != nil {
			out = append(out, role)
		}
	}
	return out
}

func (a *UserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	email, _ := data["Email"].(string)
	isActive, _ := data["IsActive"].(bool)
	plan, _ := data["Plan"].(string)
	return a.repository.Create(email, isActive, plan, a.resolveOrganization(data), a.resolveRoles(data)), nil
}

func (a *UserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	user := obj.(*User)
	email, _ := data["Email"].(string)
	isActive, _ := data["IsActive"].(bool)
	plan, _ := data["Plan"].(string)
	return a.repository.Update(user, email, isActive, plan, a.resolveOrganization(data), a.resolveRoles(data)), nil
}

func (a *UserAdmin) Delete(ctx context.Context, obj any) error {
	a.repository.Delete(obj.(*User))
	return nil
}
