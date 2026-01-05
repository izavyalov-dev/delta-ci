package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/izavyalov-dev/delta-ci/state/migrations"
)

// ApplyMigrations runs SQL migrations in order, ensuring idempotent application.
func (s *Store) ApplyMigrations(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := ensureSchemaMigrationsTable(ctx, tx); err != nil {
		return err
	}

	applied, err := loadAppliedMigrations(ctx, tx)
	if err != nil {
		return err
	}

	for _, migration := range migrations.All {
		if _, alreadyApplied := applied[migration.ID]; alreadyApplied {
			continue
		}

		if _, err := tx.ExecContext(ctx, migration.Script); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.ID, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (id, applied_at) VALUES ($1, NOW())`, migration.ID); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.ID, err)
		}
	}

	return tx.Commit()
}

func ensureSchemaMigrationsTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    id TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`)
	return err
}

func loadAppliedMigrations(ctx context.Context, tx *sql.Tx) (map[string]struct{}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		applied[id] = struct{}{}
	}

	return applied, rows.Err()
}
