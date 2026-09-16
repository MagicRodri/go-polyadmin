package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/MagicRodri/go-polyadmin/core"
)

type OrganizationAdmin struct {
	core.BaseModelAdmin
	repository *OrganizationRepository
	users      *UserRepository
}

func NewOrganizationAdmin(repository *OrganizationRepository, users *UserRepository) *OrganizationAdmin {
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
				core.NewField("Founded", core.FieldTypeDate, core.WithValidators(validFoundedDate)),
				core.NewField("Balance", core.FieldTypeDecimal, core.WithValidators(validBalance)),
			},
			// Shows each Organization's Users inline on its own
			// create/detail/edit pages -- see docs/inlines.md.
			DeclaredInlines: []core.Inline{core.NewTabularInline("users", "Organization")},
		},
		repository: repository,
		users:      users,
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

// parseOrganizationFormFields reads the two fields Task 11 added onto
// Organization out of the already-validated data map. Create/Update only
// run once Field.Validate has passed (see validFoundedDate/validBalance
// below), so a malformed value never reaches here -- but the zero value
// is still the safe fallback for an absent/optional one.
//
// core.Field.ParseFormValue (core/field.go) already coerces FieldTypeDecimal:
// it hands back a float64 when the raw string parses, and the raw string
// itself otherwise (only possible here if a validator were removed) --
// there is no equivalent coercion for FieldTypeDate, so Founded always
// arrives as the raw YYYY-MM-DD string and is parsed here.
func parseOrganizationFormFields(data map[string]any) (time.Time, float64) {
	var founded time.Time
	if s, _ := data["Founded"].(string); s != "" {
		if parsed, err := time.Parse("2006-01-02", s); err == nil {
			founded = parsed
		}
	}
	var balance float64
	switch v := data["Balance"].(type) {
	case float64:
		balance = v
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			balance = parsed
		}
	}
	return founded, balance
}

// validFoundedDate rejects a Founded value core couldn't already coerce:
// ParseFormValue does no date parsing of its own (unlike FieldTypeDecimal),
// so any non-empty string reaching here has to be checked by hand.
func validFoundedDate(ctx context.Context, value any) error {
	s, ok := value.(string)
	if !ok || s == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return errors.New("Enter a valid date.")
	}
	return nil
}

// validBalance rejects a Balance value ParseFormValue could not parse. An
// absent/empty Balance is fine -- Balance isn't WithRequired(), and
// ParseFormValue returns nil for raw == "" before it ever reaches
// FieldTypeDecimal's own parsing, so nil here always means "not
// submitted". A non-empty string, mirroring validFoundedDate's own
// empty-string check, means strconv.ParseFloat failed inside
// ParseFormValue -- a float64 is what a value that parsed becomes.
func validBalance(ctx context.Context, value any) error {
	if s, ok := value.(string); ok && s != "" {
		return errors.New("Enter a valid number.")
	}
	return nil
}

// DeletePreview: an organization's users go with it (see Delete). The
// admin shows this before anyone confirms -- docs/deletes.md.
func (a *OrganizationAdmin) DeletePreview(ctx context.Context, objects []any) (core.DeletePreview, error) {
	members := a.members(objects)
	sample := make([]any, len(members))
	for i, u := range members {
		sample[i] = u
	}
	return core.DeletePreview{Cascades: []core.DeleteGroup{{Resource: "users", Objects: sample, Total: len(members)}}}, nil
}

func (a *OrganizationAdmin) Delete(ctx context.Context, obj any) error {
	org := obj.(*Organization)
	for _, u := range a.members([]any{org}) {
		a.users.Delete(u)
	}
	a.repository.Delete(org)
	return nil
}

func (a *OrganizationAdmin) members(objects []any) []*User {
	doomed := make(map[int]bool, len(objects))
	for _, obj := range objects {
		doomed[obj.(*Organization).ID] = true
	}
	return a.users.Matching(func(u *User) bool { return u.Organization != nil && doomed[u.Organization.ID] })
}
