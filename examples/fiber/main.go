// Reference Fiber application exercising the PolyAdmin package.
//
// Run with:
//
//	go run .
//
// then open http://127.0.0.1:3000/admin
package main

import (
	"log"
	"os"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/contrib/fiber"

	"github.com/gofiber/fiber/v2"
)

func main() {
	users := NewUserRepository()
	organizations := NewOrganizationRepository()
	roles := NewRoleRepository()
	seed(users, organizations, roles)

	dashboard := newDashboard(users, organizations, roles)

	projectsDB, err := openProjectsDB()
	if err != nil {
		log.Fatal(err)
	}
	clientAdmin, projectAdmin, err := newProjectAdmins(projectsDB)
	if err != nil {
		log.Fatal(err)
	}

	// Cookie sessions over an in-memory user table (session.go). One
	// object serves as both halves: WithLoginBackend is what mounts the
	// admin's login page and makes an unauthenticated request redirect
	// to it, and WithAuthenticator is what reads the session back on
	// every subsequent request.
	sessions := NewCookieSessionBackend()
	options := []core.Option{
		core.WithModelAdmins(NewUserAdmin(users, organizations, roles), NewOrganizationAdmin(organizations, users), NewRoleAdmin(roles, users), clientAdmin, projectAdmin),
		core.WithDashboard(dashboard),
		core.WithAuthenticator(sessions),
		core.WithLoginBackend(sessions),
		// Not core.SuperuserAuthorizer: that would deny the viewer
		// account every permission, dashboard included. See session.go.
		core.WithAuthorizer(ReadOnlyForNonSuperusers{}),
		// amelie@example.com carries "fr" in Principal.Extra (session.go);
		// everyone else resolves through the switcher cookie and
		// Accept-Language instead.
		core.WithLocaleResolver(func(request any, p *core.Principal) string {
			if p == nil {
				return ""
			}
			locale, _ := p.Extra["locale"].(string)
			return locale
		}),
	}
	// en-XA, bracketed and accented, so a page can be swept for text that
	// never went through the translator.
	if os.Getenv("POLYADMIN_PSEUDO_LOCALE") == "1" {
		options = append(options, core.WithPseudoLocale())
	}
	admin := core.New(options...)
	registerPages(admin, users)

	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"admin": "/admin"})
	})

	group := app.Group("/admin")
	if err := fiberadapter.Mount(group, admin, "/admin", fiberadapter.WithTemplateDirs("templates")); err != nil {
		log.Fatal(err)
	}

	// PORT so the browser suite can run this app on its own port
	// without colliding with a development server.
	addr := ":3000"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	log.Printf("admin: http://127.0.0.1%s/admin", addr)
	log.Fatal(app.Listen(addr))
}
