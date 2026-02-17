package notify

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/state"
)

func newTestStore(t *testing.T) *state.Store {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	db.SetMaxOpenConns(4)
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	resetTables(t, ctx, db)
	t.Cleanup(func() { resetTables(t, ctx, db) })
	return store
}

func resetTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename != 'schema_migrations'
	`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, name)
	}
	for _, tbl := range tables {
		if _, err := db.ExecContext(ctx, "DELETE FROM "+tbl); err != nil {
			t.Fatalf("delete from %s: %v", tbl, err)
		}
	}
}

func createTerminalRun(t *testing.T, ctx context.Context, store *state.Store) string {
	t.Helper()
	run, err := store.CreateRun(ctx, state.Run{
		ID:        "test-run-" + t.Name(),
		RepoID:    "test-repo",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	// Transition to SUCCESS via valid path: CREATED -> PLANNING -> QUEUED -> RUNNING -> SUCCESS
	for _, next := range []state.RunState{
		state.RunStatePlanning,
		state.RunStateQueued,
		state.RunStateRunning,
		state.RunStateSuccess,
	} {
		if err := store.TransitionRunState(ctx, run.ID, next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	return run.ID
}

func createNonTerminalRun(t *testing.T, ctx context.Context, store *state.Store) string {
	t.Helper()
	run, err := store.CreateRun(ctx, state.Run{
		ID:        "test-run-nt-" + t.Name(),
		RepoID:    "test-repo",
		Ref:       "refs/heads/main",
		CommitSHA: "abc123",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	// Transition to RUNNING (non-terminal): CREATED -> PLANNING -> QUEUED -> RUNNING
	for _, next := range []state.RunState{
		state.RunStatePlanning,
		state.RunStateQueued,
		state.RunStateRunning,
	} {
		if err := store.TransitionRunState(ctx, run.ID, next); err != nil {
			t.Fatalf("transition to %s: %v", next, err)
		}
	}
	return run.ID
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
