package main

import (
	"context"
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
)

// RoleAdmin exists mainly so the User form's many-to-many has a
// registered target to resolve against: a Relation names a slug, and
// the adapter looks that slug up to turn each related object into a
// (pk, label) pair. It is a full resource in its own right all the
// same -- roles are editable like anything else.
type RoleAdmin struct {
	core.BaseModelAdmin
	repository *RoleRepository
}

func NewRoleAdmin(repository *RoleRepository) *RoleAdmin {
	return &RoleAdmin{
		BaseModelAdmin: core.BaseModelAdmin{
			ModelName:        "Role",
			NavCategory:      "Directory",
			NavIcon:          "user",
			DisplayFields:    []string{"ID", "Name"},
			FormFieldNames:   []string{"Name"},
			SearchFieldNames: []string{"Name"},
			DeclaredFields:   []core.Field{core.NewField("Name", core.FieldTypeString, core.WithRequired())},
		},
		repository: repository,
	}
}

func (a *RoleAdmin) GetQueryset(ctx context.Context) (any, error) {
	roles := a.repository.List()
	out := make([]any, len(roles))
	for i, role := range roles {
		out[i] = role
	}
	return out, nil
}

func (a *RoleAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	id, err := strconv.Atoi(pk.(string))
	if err != nil {
		return nil, nil
	}
	if role := a.repository.Get(id); role != nil {
		return role, nil
	}
	return nil, nil
}

func (a *RoleAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	name, _ := data["Name"].(string)
	return a.repository.Create(name), nil
}
