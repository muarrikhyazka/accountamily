package main

import (
	"context"
	_ "embed"
)

//go:embed schema.sql
var schemaSQL string

// initSchema runs the embedded schema.sql against the pool - idempotent
// (every statement is CREATE TABLE IF NOT EXISTS), safe to call on every boot.
func initSchema(ctx context.Context, app *App) error {
	_, err := app.DB.Exec(ctx, schemaSQL)
	return err
}
