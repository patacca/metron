// Package schemas embeds the canonical public protocol: the JSON Schemas of
// the Metron contract, report, packet and ledger event, version 1.
package schemas

import "embed"

// FS holds the schema files under v1/.
//
//go:embed v1/*.schema.json
var FS embed.FS
