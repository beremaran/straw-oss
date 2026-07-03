// Package migrations embeds the SQL migration files that Control applies to
// Postgres at startup, so the running binary carries its schema regardless of
// working directory.
package migrations

import "embed"

// Postgres holds the Postgres migration SQL files under postgres/.
//
//go:embed postgres/*.sql
var Postgres embed.FS
