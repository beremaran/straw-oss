package migrations

import "embed"

// FS is the embedded filesystem containing migration SQL files.
//
//go:embed *.sql
var FS embed.FS
