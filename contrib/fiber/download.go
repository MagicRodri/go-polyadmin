package fiber

import (
	"fmt"
	"strings"

	"github.com/MagicRodri/go-polyadmin/core"

	"github.com/gofiber/fiber/v2"
)

// rfc5987Safe is the attr-char punctuation RFC 5987 lets through unescaped.
const rfc5987Safe = "!#$&+-.^_`|~"

// contentDisposition is an attachment header carrying the name twice: an
// ASCII fallback for old clients, and the RFC 5987 filename* every current
// browser prefers.
func contentDisposition(filename string) string {
	var fallback, encoded strings.Builder
	for _, r := range filename {
		if r >= 0x20 && r < 0x7f && r != '"' && r != '\\' {
			fallback.WriteRune(r)
		}
	}
	for _, b := range []byte(filename) {
		alnum := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
		if alnum || strings.IndexByte(rfc5987Safe, b) >= 0 {
			encoded.WriteByte(b)
		} else {
			fmt.Fprintf(&encoded, "%%%02X", b)
		}
	}
	name := fallback.String()
	if name == "" {
		name = "download"
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, name, encoded.String())
}

func sendDownload(c *fiber.Ctx, d core.Download) error {
	contentType := d.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderContentDisposition, contentDisposition(d.Filename))
	if d.Stream != nil {
		return c.SendStream(d.Stream)
	}
	return c.Send(d.Content)
}
