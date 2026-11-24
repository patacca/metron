// Package protocol embeds the canonical text of the Metron Engineering Review
// Protocol, version metron.protocol/v1, and the templates from which role
// briefs are rendered.
package protocol

import "embed"

// FS holds the protocol text, v1/protocol.md, and the Go text/template files
// rendered by internal/brief: one brief per role under v1/roles/, the
// partials they share in v1/common.md.tmpl, and the layout of a
// self-contained prompt in v1/prompt.md.tmpl.
//
//go:embed v1/protocol.md v1/*.tmpl v1/roles/*.tmpl
var FS embed.FS
