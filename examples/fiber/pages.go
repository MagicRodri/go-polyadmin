// A custom admin page -- demonstrates admin.Route() for functionality
// that isn't resource CRUD (a department-facing wizard, a report, an
// internal tool). See docs/routing.md.
package main

import (
	"strconv"

	"github.com/MagicRodri/go-polyadmin/core"
	fiberadapter "github.com/MagicRodri/go-polyadmin/fiber"

	"github.com/gofiber/fiber/v2"
)

type broadcastPageData struct {
	Error string
}

func registerPages(admin *core.Admin, users *UserRepository) {
	admin.Route(
		"/tools/broadcast",
		fiberadapter.PageHandler(broadcastHandler(users)),
		core.WithPageLabel("Broadcast Message"),
		core.WithPageCategory("Tools"),
	)
}

func broadcastHandler(users *UserRepository) fiberadapter.PageHandler {
	return func(pc *fiberadapter.PageContext) error {
		if pc.C.Method() == fiber.MethodPost {
			message := pc.C.FormValue("message")
			if message == "" {
				return pc.Render("pages/broadcast.html", broadcastPageData{Error: "Message can't be empty."})
			}
			recipients := 0
			for _, u := range users.List() {
				if u.IsActive {
					recipients++
				}
			}
			return pc.RedirectWithFlash(
				pc.BasePath+"/tools/broadcast", "success",
				"Broadcast sent to "+strconv.Itoa(recipients)+" active user(s).",
			)
		}
		return pc.Render("pages/broadcast.html", broadcastPageData{})
	}
}
