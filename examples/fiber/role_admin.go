package main

import (
	"context"
	"fmt"
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
	users      *UserRepository
}

func NewRoleAdmin(repository *RoleRepository, users *UserRepository) *RoleAdmin {
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
		users:      users,
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

// DeletePreview: a role still held by anyone is protected -- it cannot be
// deleted until those users no longer hold it (docs/deletes.md).
func (a *RoleAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
	holders := a.holders(objects)
	sample := make([]any, len(holders))
	for i, u := range holders {
		sample[i] = u
	}
	return core.DeletePreview{Protected: []core.DeleteGroup{{Resource: "users", Objects: sample, Total: len(holders)}}}, nil
}

// Delete refuses too: the preview is what the admin shows, but storage
// keeps its own rule, as a foreign-key constraint would.
func (a *RoleAdmin) Delete(ctx context.Context, obj any) error {
	role := obj.(*Role)
	if len(a.holders([]any{role})) > 0 {
		return fmt.Errorf("role %q is still assigned", role.Name)
	}
	a.repository.Delete(role)
	return nil
}

func (a *RoleAdmin) holders(objects []any) []*User {
	doomed := make(map[int]bool, len(objects))
	for _, obj := range objects {
		doomed[obj.(*Role).ID] = true
	}
	return a.users.Matching(func(u *User) bool {
		for _, r := range u.Roles {
			if role, ok := r.(*Role); ok && doomed[role.ID] {
				return true
			}
		}
		return false
	})
}
