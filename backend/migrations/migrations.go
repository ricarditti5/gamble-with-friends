// Package migrations embeds the SQL migration files (up/down pairs).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
