// Package migrations applies embedded, immutable SQL migrations atomically.
package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"fmt"
	"strings"
)

//go:embed *.sql
var files embed.FS

func Up(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(718209431); CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT clock_timestamp())`); err != nil {
		return err
	}
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := files.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(body))
		var existing string
		err = tx.QueryRowContext(ctx, `SELECT checksum FROM schema_migrations WHERE version=$1`, entry.Name()).Scan(&existing)
		if err == nil {
			if sum != existing {
				return fmt.Errorf("migration %s checksum changed; a local development database can be recreated with make db-reset or scripts/run-local.ps1 -Reset", entry.Name())
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version,checksum) VALUES ($1,$2)`, entry.Name(), sum); err != nil {
			return err
		}
	}
	return tx.Commit()
}
