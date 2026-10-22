// Package migrations embeds the SQL migrations applied by `tensora migrate` (goose format).
// Add a migration: create NNNNN_description.sql with -- +goose Up / -- +goose Down sections.
// Never edit a migration that has been applied anywhere; add a new one.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
