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
			// The RadioGroup/Switch/Slider on the page post like any
			// other form control -- each is a native input underneath
			// (or, for the Switch, a hidden input Alpine writes to), so
			// there's no JSON body or client-side state to unpack here.
			urgency := pc.C.FormValue("urgency")
			if urgency == "" {
				urgency = "normal"
			}
			channels := "in-app"
			if pc.C.FormValue("also_email") == "true" {
				channels = "in-app + email"
			}
			rate := pc.C.FormValue("rate")
			if rate == "" {
				rate = "100"
			}
			return pc.RedirectWithFlash(
				pc.BasePath+"/tools/broadcast", "success",
				"Broadcast sent to "+strconv.Itoa(recipients)+" active user(s) ("+
					urgency+" urgency, "+channels+", "+rate+"/min).",
			)
		}
		return pc.Render("pages/broadcast.html", broadcastPageData{})
	}
}
