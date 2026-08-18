// Package templates embeds the framework's html/template files so the
// Fiber adapter ships them compiled into the binary -- no separate
// asset directory to deploy alongside it.
package templates

import "embed"

//go:embed admin
var FS embed.FS
