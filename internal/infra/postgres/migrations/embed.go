// Package migrations provides the embedded SQL migration files for the postgres database.
package migrations

import "embed"

// FS contains all embedded migration SQL files.
//
//go:embed *.sql
var FS embed.FS
