// Package accountmigrations provides embedded SQL migration files for the account schema.
package accountmigrations

import "embed"

// FS contains the embedded SQL migration files.
// Files follow golang-migrate naming: NNNNNN_description.up.sql / .down.sql
//
//go:embed *.sql
var FS embed.FS
