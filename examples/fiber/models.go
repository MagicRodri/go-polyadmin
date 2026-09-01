package main

// In-memory Organization/User models + repositories for the reference
// app. A real application would back these with GORM, sqlx, Bun, or a
// repository over its own database -- the admin core
// doesn't care which. Kept intentionally simple here since the point
// of this example is to exercise the PolyAdmin package, not demonstrate
// an ORM.

type Organization struct {
	ID   int
	Name string
}

type OrganizationRepository struct {
	organizations map[int]*Organization
	nextID        int
}

func NewOrganizationRepository() *OrganizationRepository {
	return &OrganizationRepository{organizations: make(map[int]*Organization), nextID: 1}
}

func (r *OrganizationRepository) List() []*Organization {
	out := make([]*Organization, 0, len(r.organizations))
	for _, o := range r.organizations {
		out = append(out, o)
	}
	return out
}

func (r *OrganizationRepository) Get(pk int) *Organization {
	return r.organizations[pk]
}

func (r *OrganizationRepository) Create(name string) *Organization {
	o := &Organization{ID: r.nextID, Name: name}
	r.organizations[o.ID] = o
	r.nextID++
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

func (r *UserRepository) List() []*User {
	out := make([]*User, 0, len(r.users))
	for _, u := range r.users {
		out = append(out, u)
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

func seed(users *UserRepository, organizations *OrganizationRepository, roles *RoleRepository) {
	acme := organizations.Create("Acme Corp")
	widgets := organizations.Create("Widgets Inc")
	globex := organizations.Create("Globex Corporation")
	initech := organizations.Create("Initech")

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
}
