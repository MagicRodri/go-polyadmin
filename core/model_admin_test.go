package core

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type User struct {
	ID       int
	Email    string
	IsActive bool
}

type inMemoryUserAdmin struct {
	BaseModelAdmin
	users  *[]*User
	nextID *int
}

func newInMemoryUserAdmin() inMemoryUserAdmin {
	users := []*User{}
	nextID := 1
	return inMemoryUserAdmin{
		BaseModelAdmin: BaseModelAdmin{
			ModelName:        "User",
			DisplayFields:    []string{"ID", "Email", "IsActive"},
			FormFieldNames:   []string{"Email", "IsActive"},
			SearchFieldNames: []string{"Email"},
			DeclaredFields: []Field{
				NewField("IsActive", FieldTypeBoolean, WithDefault(true)),
				NewField("Email", FieldTypeEmail, WithRequired()),
			},
		},
		users:  &users,
		nextID: &nextID,
	}
}

func (a inMemoryUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	return *a.users, nil
}

func (a inMemoryUserAdmin) GetObject(ctx context.Context, pk any) (any, error) {
	for _, u := range *a.users {
		if u.ID == pk {
			return u, nil
		}
	}
	return nil, nil
}

func (a inMemoryUserAdmin) Create(ctx context.Context, data map[string]any) (any, error) {
	u := &User{ID: *a.nextID, IsActive: true}
	if email, ok := data["Email"].(string); ok {
		u.Email = email
	}
	if active, ok := data["IsActive"].(bool); ok {
		u.IsActive = active
	}
	*a.users = append(*a.users, u)
	*a.nextID++
	return u, nil
}

func (a inMemoryUserAdmin) Update(ctx context.Context, obj any, data map[string]any) (any, error) {
	u := obj.(*User)
	if email, ok := data["Email"].(string); ok {
		u.Email = email
	}
	if active, ok := data["IsActive"].(bool); ok {
		u.IsActive = active
	}
	return u, nil
}

func (a inMemoryUserAdmin) Delete(ctx context.Context, obj any) error {
	u := obj.(*User)
	filtered := (*a.users)[:0]
	for _, existing := range *a.users {
		if existing.ID != u.ID {
			filtered = append(filtered, existing)
		}
	}
	*a.users = filtered
	return nil
}

func TestSlugDefaultsFromModelName(t *testing.T) {
	if got := newInMemoryUserAdmin().Slug(); got != "users" {
		t.Fatalf("got %q", got)
	}
}

func TestSlugCanBeOverridden(t *testing.T) {
	admin := newInMemoryUserAdmin()
	admin.SlugOverride = "people"
	if got := admin.Slug(); got != "people" {
		t.Fatalf("got %q", got)
	}
}

func TestCategoryDefaultsToEmpty(t *testing.T) {
	if got := newInMemoryUserAdmin().Category(); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestCategoryCanBeSet(t *testing.T) {
	admin := newInMemoryUserAdmin()
	admin.NavCategory = "Directory"
	if got := admin.Category(); got != "Directory" {
		t.Fatalf("got %q", got)
	}
}

func TestIconDefaultsToCollection(t *testing.T) {
	if got := newInMemoryUserAdmin().Icon(); got != "collection" {
		t.Fatalf("got %q", got)
	}
}

func TestIconCanBeSet(t *testing.T) {
	admin := newInMemoryUserAdmin()
	admin.NavIcon = "table"
	if got := admin.Icon(); got != "table" {
		t.Fatalf("got %q", got)
	}
}

func TestFieldsAreResolvedFromDisplayAndFormFields(t *testing.T) {
	admin := newInMemoryUserAdmin()
	fields := admin.Fields()

	gotNames := make(map[string]bool, len(fields))
	for name := range fields {
		gotNames[name] = true
	}
	want := map[string]bool{"ID": true, "Email": true, "IsActive": true}
	if !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("got %v, want %v", gotNames, want)
	}

	// explicitly declared field is used as-is, not overwritten by the implicit default
	if fields["IsActive"].Default != true {
		t.Fatalf("got default %v", fields["IsActive"].Default)
	}
}

func TestListDisplayValues(t *testing.T) {
	admin := newInMemoryUserAdmin()
	obj, _ := admin.Create(context.Background(), map[string]any{"Email": "john@example.com", "IsActive": false})
	user := obj.(*User)

	got := admin.ListDisplayValues(user)
	want := map[string]any{"ID": 1, "Email": "john@example.com", "IsActive": false}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCRUDLifecycle(t *testing.T) {
	ctx := context.Background()
	admin := newInMemoryUserAdmin()

	obj, _ := admin.Create(ctx, map[string]any{"Email": "john@example.com"})
	user := obj.(*User)

	got, _ := admin.GetObject(ctx, user.ID)
	if got != user {
		t.Fatalf("GetObject returned a different instance")
	}

	qs, _ := admin.GetQueryset(ctx)
	if users := qs.([]*User); len(users) != 1 || users[0] != user {
		t.Fatalf("got %v", users)
	}

	admin.Update(ctx, user, map[string]any{"Email": "john2@example.com"})
	if user.Email != "john2@example.com" {
		t.Fatalf("got %q", user.Email)
	}

	admin.Delete(ctx, user)
	qs, _ = admin.GetQueryset(ctx)
	if users := qs.([]*User); len(users) != 0 {
		t.Fatalf("got %v", users)
	}
}

func TestUnimplementedCRUDReturnsError(t *testing.T) {
	var admin BaseModelAdmin
	ctx := context.Background()

	if _, err := admin.GetQueryset(ctx); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
	if _, err := admin.GetObject(ctx, 1); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
	if _, err := admin.Create(ctx, nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
	if _, err := admin.Update(ctx, nil, nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
	if err := admin.Delete(ctx, nil); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("got %v", err)
	}
}

func TestValidateRequiredField(t *testing.T) {
	admin := newInMemoryUserAdmin()
	errs := admin.Validate(map[string]any{"Email": ""})
	if _, ok := errs["Email"]; !ok {
		t.Fatalf("got %v, want an Email error", errs)
	}
}
