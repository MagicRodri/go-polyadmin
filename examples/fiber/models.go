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

type User struct {
	ID           int
	Email        string
	IsActive     bool
	Organization *Organization
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

func (r *UserRepository) Create(email string, isActive bool, organization *Organization) *User {
	u := &User{ID: r.nextID, Email: email, IsActive: isActive, Organization: organization}
	r.users[u.ID] = u
	r.nextID++
	return u
}

func (r *UserRepository) Update(u *User, email string, isActive bool, organization *Organization) *User {
	u.Email = email
	u.IsActive = isActive
	u.Organization = organization
	return u
}

func (r *UserRepository) Delete(u *User) {
	delete(r.users, u.ID)
}

func seed(users *UserRepository, organizations *OrganizationRepository) {
	acme := organizations.Create("Acme Corp")
	widgets := organizations.Create("Widgets Inc")
	globex := organizations.Create("Globex Corporation")
	initech := organizations.Create("Initech")
	users.Create("admin@example.com", true, acme)
	users.Create("jane@example.com", true, acme)
	users.Create("john@example.com", false, widgets)
	users.Create("mary@example.com", true, widgets)
	users.Create("peter@example.com", true, globex)
	users.Create("samir@example.com", true, initech)
	users.Create("milton@example.com", false, nil)
}
