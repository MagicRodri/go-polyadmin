package core

import (
	"context"
	"reflect"
	"testing"
)

// Distinct names from relation_test.go's/model_admin_test.go's own
// test doubles (same package) -- these are inline_test.go's own
// self-contained parent/child pair.

type inlineOrg struct {
	ID   int
	Name string
}

type inlineUser struct {
	ID           int
	Email        string
	Organization *inlineOrg
}

type inlineOrgAdmin struct {
	BaseModelAdmin
	orgs []*inlineOrg
}

func (a inlineOrgAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, len(a.orgs))
	for i, o := range a.orgs {
		out[i] = o
	}
	return out, nil
}

func newInlineOrgAdmin(orgs ...*inlineOrg) inlineOrgAdmin {
	return inlineOrgAdmin{
		BaseModelAdmin: BaseModelAdmin{ModelName: "Organization", SlugOverride: "organizations"},
		orgs:           orgs,
	}
}

type inlineUserAdmin struct {
	BaseModelAdmin
	users []*inlineUser
}

func (a inlineUserAdmin) GetQueryset(ctx context.Context) (any, error) {
	out := make([]any, len(a.users))
	for i, u := range a.users {
		out[i] = u
	}
	return out, nil
}

func newInlineUserAdmin(users ...*inlineUser) inlineUserAdmin {
	organizationRelation := Relation{Name: "Organization", Target: "organizations"}
	return inlineUserAdmin{
		BaseModelAdmin: BaseModelAdmin{
			ModelName:      "User",
			DisplayFields:  []string{"ID", "Email", "Organization"},
			FormFieldNames: []string{"Email", "Organization"},
			DeclaredFields: []Field{
				NewField("Email", FieldTypeString, WithRequired()),
				NewField("Organization", FieldTypeForeignKey, WithRelation(organizationRelation)),
			},
		},
		users: users,
	}
}

func TestNewStackedInlineDefaults(t *testing.T) {
	inline := NewStackedInline("users", "Organization")
	if inline.Child != "users" || inline.FKField != "Organization" || inline.Layout != InlineLayoutStacked || inline.Label != "" {
		t.Fatalf("got %+v", inline)
	}
}

func TestNewTabularInlineSetsLayout(t *testing.T) {
	inline := NewTabularInline("users", "Organization")
	if inline.Layout != InlineLayoutTabular {
		t.Fatalf("got %q", inline.Layout)
	}
}

func TestInlineLabelOptionOverrides(t *testing.T) {
	inline := NewStackedInline("users", "Organization", WithInlineLabel("Members"))
	if inline.Label != "Members" {
		t.Fatalf("got %q", inline.Label)
	}
}

func TestFilterInlineChildrenMatchesByParentPK(t *testing.T) {
	acme := &inlineOrg{ID: 1, Name: "Acme"}
	widgets := &inlineOrg{ID: 2, Name: "Widgets"}
	orgAdmin := newInlineOrgAdmin(acme, widgets)
	userAdmin := newInlineUserAdmin(
		&inlineUser{ID: 1, Email: "a@example.com", Organization: acme},
		&inlineUser{ID: 2, Email: "b@example.com", Organization: widgets},
		&inlineUser{ID: 3, Email: "c@example.com", Organization: acme},
	)

	result, err := FilterInlineChildren(context.Background(), userAdmin, "Organization", orgAdmin, acme.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var emails []string
	for _, obj := range result {
		emails = append(emails, obj.(*inlineUser).Email)
	}
	if !reflect.DeepEqual(emails, []string{"a@example.com", "c@example.com"}) {
		t.Fatalf("got %v", emails)
	}
}

func TestFilterInlineChildrenNoMatchReturnsEmpty(t *testing.T) {
	acme := &inlineOrg{ID: 1, Name: "Acme"}
	orgAdmin := newInlineOrgAdmin(acme)
	userAdmin := newInlineUserAdmin(&inlineUser{ID: 1, Email: "a@example.com", Organization: acme})

	result, err := FilterInlineChildren(context.Background(), userAdmin, "Organization", orgAdmin, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %v", result)
	}
}

func TestFilterInlineChildrenSkipsUnsetFK(t *testing.T) {
	acme := &inlineOrg{ID: 1, Name: "Acme"}
	orgAdmin := newInlineOrgAdmin(acme)
	userAdmin := newInlineUserAdmin(&inlineUser{ID: 1, Email: "a@example.com", Organization: nil})

	result, err := FilterInlineChildren(context.Background(), userAdmin, "Organization", orgAdmin, acme.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("got %v", result)
	}
}

func TestFilterInlineChildrenComparesPKsAsStrings(t *testing.T) {
	acme := &inlineOrg{ID: 1, Name: "Acme"}
	orgAdmin := newInlineOrgAdmin(acme)
	userAdmin := newInlineUserAdmin(&inlineUser{ID: 1, Email: "a@example.com", Organization: acme})

	// parentPK passed as a string (as it would arrive from a URL path
	// param) must still match the int ID stored on the object.
	result, err := FilterInlineChildren(context.Background(), userAdmin, "Organization", orgAdmin, "1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %v", result)
	}
}

func TestFilterInlineChildrenUnknownFieldReturnsError(t *testing.T) {
	orgAdmin := newInlineOrgAdmin()
	userAdmin := newInlineUserAdmin()

	if _, err := FilterInlineChildren(context.Background(), userAdmin, "NoSuchField", orgAdmin, 1); err == nil {
		t.Fatalf("expected an error for an unknown fk field")
	}
}
