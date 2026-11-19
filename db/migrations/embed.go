// Package migrations embeds the Goose SQL migrations so binaries carry their own schema.
//
// Rules: forward-only in QA/PROD; incompatible changes follow expand → migrate → contract
// across releases (docs/architecture/database.md §4).
package migrations

import "embed"

// FS holds every *.sql migration.
//
//go:embed *.sql
var FS embed.FS
