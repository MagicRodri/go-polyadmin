package main

import (
	"fmt"
	"time"
)

// In-memory Organization/User models + repositories for the reference
// app. A real application would back these with GORM, sqlx, Bun, or a
// repository over its own database -- the admin core
// doesn't care which. Kept intentionally simple here since the point
// of this example is to exercise the PolyAdmin package, not demonstrate
// an ORM.

type Organization struct {
	ID      int
	Name    string
	Founded time.Time
	Balance float64
}

type OrganizationRepository struct {
	organizations map[int]*Organization
	nextID        int
}

func NewOrganizationRepository() *OrganizationRepository {
	return &OrganizationRepository{organizations: make(map[int]*Organization), nextID: 1}
}

// List returns organizations in insertion order, for the same reason
// UserRepository.List does.
func (r *OrganizationRepository) List() []*Organization {
	out := make([]*Organization, 0, len(r.organizations))
	for id := 1; id < r.nextID; id++ {
		if o := r.organizations[id]; o != nil {
			out = append(out, o)
		}
	}
	return out
}

func (r *OrganizationRepository) Get(pk int) *Organization {
	return r.organizations[pk]
}

func (r *OrganizationRepository) Create(name string, founded time.Time, balance float64) *Organization {
	o := &Organization{ID: r.nextID, Name: name, Founded: founded, Balance: balance}
	r.organizations[o.ID] = o
	r.nextID++
	return o
}

func (r *OrganizationRepository) Delete(o *Organization) { delete(r.organizations, o.ID) }

func (r *OrganizationRepository) Update(o *Organization, name string, founded time.Time, balance float64) *Organization {
	o.Name = name
	o.Founded = founded
	o.Balance = balance
	return o
}

// Role is the many-to-many target: a user holds any number of them.
// Modelled after the permissions list Django's admin is known for, and
// what the searchable multi-select on the user form is there to make
// bearable once the list is long.
type Role struct {
	ID   int
	Name string
}

type RoleRepository struct {
	roles  map[int]*Role
	nextID int
}

func NewRoleRepository() *RoleRepository {
	return &RoleRepository{roles: make(map[int]*Role), nextID: 1}
}

// List returns roles in insertion order. The other repositories here
// range over their map directly and so reorder between requests, which
// a table shrugs off; a searchable option list that reshuffles under
// the cursor on every page load does not.
func (r *RoleRepository) List() []*Role {
	out := make([]*Role, 0, len(r.roles))
	for id := 1; id < r.nextID; id++ {
		if role := r.roles[id]; role != nil {
			out = append(out, role)
		}
	}
	return out
}

func (r *RoleRepository) Get(pk int) *Role { return r.roles[pk] }

func (r *RoleRepository) Delete(role *Role) { delete(r.roles, role.ID) }

func (r *RoleRepository) Create(name string) *Role {
	role := &Role{ID: r.nextID, Name: name}
	r.roles[role.ID] = role
	r.nextID++
	return role
}

type User struct {
	ID       int
	Email    string
	IsActive bool
	// A plain choice field, so the reference app exercises ui/select
	// (the shadcn Select port). Every other choice-shaped field here is
	// a relation, which renders one of the two combobox widgets
	// instead -- without this, ui/select appeared nowhere in the app.
	Plan         string
	Organization *Organization
	// []any, not []*Role: the Fiber adapter reads a many-to-many's
	// current value with a `.([]any)` assertion, the same "collections
	// are []any" convention its GetQueryset note spells out. A typed
	// slice here asserts to nothing and the field would render as an
	// empty selection.
	Roles []any
}

type UserRepository struct {
	users  map[int]*User
	nextID int
}

func NewUserRepository() *UserRepository {
	return &UserRepository{users: make(map[int]*User), nextID: 1}
}

// List returns users in insertion order, like RoleRepository.List and for
// the same reason: ranging over the map reshuffles between requests, which
// a page of 25 out of 200 makes obvious -- the same reload shows a
// different page.
func (r *UserRepository) List() []*User {
	out := make([]*User, 0, len(r.users))
	for id := 1; id < r.nextID; id++ {
		if u := r.users[id]; u != nil {
			out = append(out, u)
		}
	}
	return out
}

func (r *UserRepository) Get(pk int) *User {
	return r.users[pk]
}

func (r *UserRepository) Create(email string, isActive bool, plan string, organization *Organization, roles []any) *User {
	u := &User{ID: r.nextID, Email: email, IsActive: isActive, Plan: plan, Organization: organization, Roles: roles}
	r.users[u.ID] = u
	r.nextID++
	return u
}

func (r *UserRepository) Update(u *User, email string, isActive bool, plan string, organization *Organization, roles []any) *User {
	u.Email = email
	u.IsActive = isActive
	u.Plan = plan
	u.Organization = organization
	u.Roles = roles
	return u
}

func (r *UserRepository) Delete(u *User) {
	delete(r.users, u.ID)
}

// Matching returns the users keep accepts, in ID order -- the repository
// is a map, and a preview's sample should not reshuffle between requests.
func (r *UserRepository) Matching(keep func(*User) bool) []*User {
	var out []*User
	for id := 1; id < r.nextID; id++ {
		if u := r.users[id]; u != nil && keep(u) {
			out = append(out, u)
		}
	}
	return out
}

func seed(users *UserRepository, organizations *OrganizationRepository, roles *RoleRepository) {
	// Founded in the same month for every organization: Task 15's browser
	// test checks that this date renders with a French month name under
	// the fr locale, and it doesn't matter which organization it looks at.
	// Acme keeps March 2019: a browser test pins "Mar 1, 2019" as a data
	// value and checks the same date renders "mars" under fr. The others
	// spread out, so the date drill-down has years and months to walk.
	founded := time.Date(2019, 3, 1, 0, 0, 0, 0, time.UTC)
	acme := organizations.Create("Acme Corp", founded, 1234.5)
	widgets := organizations.Create("Widgets Inc", time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC), 1234.5)
	globex := organizations.Create("Globex Corporation", time.Date(2023, 11, 2, 0, 0, 0, 0, time.UTC), 1234.5)
	initech := organizations.Create("Initech", time.Date(2023, 11, 20, 0, 0, 0, 0, time.UTC), 1234.5)
	for i := 5; i < 25; i++ {
		organizations.Create(
			fmt.Sprintf("Org %02d Holdings", i),
			time.Date(2019+i%5, time.Month(1+(i*3)%12), 1+(i*7)%28, 0, 0, 0, 0, time.UTC),
			float64(1000+i*137),
		)
	}

	// Enough roles that the multi-select's search box has something to
	// do -- the control only earns its keep past the point where
	// scanning the whole list stops being quick.
	admin := roles.Create("Administrator")
	billing := roles.Create("Billing")
	support := roles.Create("Support")
	roles.Create("Auditor")
	roles.Create("Content Editor")
	roles.Create("Release Manager")
	roles.Create("Read Only")
	security := roles.Create("Security Officer")

	users.Create("admin@example.com", true, "Enterprise", acme, []any{admin, security})
	users.Create("jane@example.com", true, "Pro", acme, []any{billing})
	users.Create("john@example.com", false, "Free", widgets, nil)
	users.Create("mary@example.com", true, "Pro", widgets, []any{support, billing})
	users.Create("peter@example.com", true, "Enterprise", globex, []any{support})
	users.Create("samir@example.com", true, "Free", initech, nil)
	users.Create("milton@example.com", false, "Free", nil, nil)

	// Enough rows to fill a viewport and give pagination, filters and the
	// date drill-down something to work on. The seven named users above are
	// what the tests assert against, so the filler deliberately avoids
	// Initech (whose cascade count is asserted) and the Auditor role (whose
	// being unheld is asserted), and is generated rather than listed.
	plans := []string{"Free", "Pro", "Enterprise"}
	fillerOrgs := []*Organization{acme, widgets, globex, nil}
	fillerRoles := [][]any{nil, {support}, {billing}, {admin}, {security, support}}
	for i := len(users.List()) + 1; i <= 200; i++ {
		users.Create(
			fmt.Sprintf("user%03d@example.com", i),
			i%4 != 0,
			plans[i%len(plans)],
			fillerOrgs[i%len(fillerOrgs)],
			fillerRoles[i%len(fillerRoles)],
		)
	}
}
