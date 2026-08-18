package fiber

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"
)

// flashMessage is a short-lived, cookie-carried notice that survives a
// redirect and is rendered as a toast by the page it lands
// on -- mirrors the Python adapter's admin/fastapi/responses.py.
type flashMessage struct {
	Level string `json:"level"`
	Text  string `json:"text"`
}

const flashCookieName = "admin_messages"

func setFlash(c *fiber.Ctx, level, text string) {
	raw, err := json.Marshal([]flashMessage{{Level: level, Text: text}})
	if err != nil {
		return
	}
	c.Cookie(&fiber.Cookie{
		Name:     flashCookieName,
		Value:    string(raw),
		MaxAge:   10,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

// popFlash reads pending flash messages without clearing the cookie --
// callers should call clearFlash on whichever response they end up
// sending, once they know they've consumed the messages.
func popFlash(c *fiber.Ctx) []flashMessage {
	raw := c.Cookies(flashCookieName)
	if raw == "" {
		return nil
	}
	var messages []flashMessage
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil
	}
	return messages
}

func clearFlash(c *fiber.Ctx) {
	c.Cookie(&fiber.Cookie{
		Name:     flashCookieName,
		Value:    "",
		MaxAge:   -1,
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}
