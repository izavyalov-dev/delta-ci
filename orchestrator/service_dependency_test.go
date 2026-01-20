package orchestrator

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func TestDependencyGatingQueuesDependents(t *testing.T) {
	ctx := context.Background()
	store, cleanup := setupTestStore(t, ctx)
	defer cleanup()

	dispatcher := &recordingDispatcher{}
	plan := stubPlanner{
		jobs: []planner.PlannedJob{
			{
				Name:     "build",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "build",
					Workdir: ".",
					Steps:   []string{"echo build"},
				},
				Reason: "stub build",
			},
			{
				Name:      "test",
				Required:  true,
				DependsOn: []string{"build"},
				Spec: protocol.JobSpec{
					Name:    "test",
					Workdir: ".",
					Steps:   []string{"echo test"},
				},
				Reason: "stub test",
			},
			{
				Name:      "lint",
				Required:  false,
				DependsOn: []string{"build"},
				Spec: protocol.JobSpec{
					Name:    "lint",
					Workdir: ".",
					Steps:   []string{"echo lint"},
				},
				Reason: "stub lint",
			},
		},
	}
	service := NewService(store, plan, dispatcher, &sequenceIDGen{}, nil, nil)

	details, err := service.CreateRun(ctx, CreateRunRequest{
		RepoID:    "repo",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}

	jobs, err := store.ListJobsByRun(ctx, details.Run.ID)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	jobByName := map[string]state.Job{}
	for _, job := range jobs {
		jobByName[job.Name] = job
	}

	buildJob, ok := jobByName["build"]
	if !ok {
		t.Fatalf("missing build job")
	}
	testJob, ok := jobByName["test"]
	if !ok {
		t.Fatalf("missing test job")
	}
	lintJob, ok := jobByName["lint"]
	if !ok {
		t.Fatalf("missing lint job")
	}

	if buildJob.State != state.JobStateQueued {
		t.Fatalf("expected build queued, got %s", buildJob.State)
	}
	if testJob.State != state.JobStateCreated {
		t.Fatalf("expected test created, got %s", testJob.State)
	}
	if lintJob.State != state.JobStateCreated {
		t.Fatalf("expected lint created, got %s", lintJob.State)
	}

	buildAttempt := latestAttemptForJob(t, ctx, store, buildJob.ID)
	testAttempt := latestAttemptForJob(t, ctx, store, testJob.ID)
	lintAttempt := latestAttemptForJob(t, ctx, store, lintJob.ID)

	if buildAttempt.State != state.JobStateQueued {
		t.Fatalf("expected build attempt queued, got %s", buildAttempt.State)
	}
	if testAttempt.State != state.JobStateCreated {
		t.Fatalf("expected test attempt created, got %s", testAttempt.State)
	}
	if lintAttempt.State != state.JobStateCreated {
		t.Fatalf("expected lint attempt created, got %s", lintAttempt.State)
	}

	if len(dispatcher.attempts) != 1 {
		t.Fatalf("expected 1 queued attempt, got %d", len(dispatcher.attempts))
	}

	transitionJobToSucceeded(t, ctx, service, buildJob.ID, buildAttempt.ID)

	if err := service.enqueueReadyDependents(ctx, buildJob); err != nil {
		t.Fatalf("enqueue dependents: %v", err)
	}

	if len(dispatcher.attempts) != 3 {
		t.Fatalf("expected 3 queued attempts, got %d", len(dispatcher.attempts))
	}

	testJob, err = store.GetJob(ctx, testJob.ID)
	if err != nil {
		t.Fatalf("get test job: %v", err)
	}
	lintJob, err = store.GetJob(ctx, lintJob.ID)
	if err != nil {
		t.Fatalf("get lint job: %v", err)
	}
	if testJob.State != state.JobStateQueued {
		t.Fatalf("expected test queued, got %s", testJob.State)
	}
	if lintJob.State != state.JobStateQueued {
		t.Fatalf("expected lint queued, got %s", lintJob.State)
	}
}

type recordingDispatcher struct {
	attempts []state.JobAttempt
}

func (d *recordingDispatcher) EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error {
	d.attempts = append(d.attempts, attempt)
	return nil
}

type stubPlanner struct {
	jobs []planner.PlannedJob
}

func (p stubPlanner) Plan(ctx context.Context, req planner.PlanRequest) (planner.PlanResult, error) {
	return planner.PlanResult{
		Jobs:    p.jobs,
		Explain: "stub plan",
	}, nil
}

type sequenceIDGen struct {
	run     int
	job     int
	attempt int
	lease   int
}

func (g *sequenceIDGen) RunID() string {
	g.run++
	return fmt.Sprintf("run-%d", g.run)
}

func (g *sequenceIDGen) JobID() string {
	g.job++
	return fmt.Sprintf("job-%d", g.job)
}

func (g *sequenceIDGen) JobAttemptID() string {
	g.attempt++
	return fmt.Sprintf("attempt-%d", g.attempt)
}

func (g *sequenceIDGen) LeaseID() string {
	g.lease++
	return fmt.Sprintf("lease-%d", g.lease)
}

func latestAttemptForJob(t *testing.T, ctx context.Context, store *state.Store, jobID string) state.JobAttempt {
	t.Helper()
	attempts, err := store.ListJobAttempts(ctx, jobID)
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if len(attempts) == 0 {
		t.Fatalf("no attempts for job %s", jobID)
	}
	return attempts[len(attempts)-1]
}

func transitionJobToSucceeded(t *testing.T, ctx context.Context, service *Service, jobID, attemptID string) {
	t.Helper()
	steps := []state.JobState{
		state.JobStateLeased,
		state.JobStateStarting,
		state.JobStateRunning,
		state.JobStateUploading,
		state.JobStateSucceeded,
	}
	for _, step := range steps {
		if err := service.transitionJobAndAttempt(ctx, jobID, attemptID, step); err != nil {
			t.Fatalf("transition to %s: %v", step, err)
		}
	}
	if err := service.store.MarkJobAttemptCompleted(ctx, attemptID, time.Now().UTC()); err != nil {
		t.Fatalf("mark attempt completed: %v", err)
	}
}

func setupTestStore(t *testing.T, ctx context.Context) (*state.Store, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(4)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping db: %v", err)
	}

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("apply migrations: %v", err)
	}
	if err := resetDatabase(ctx, db); err != nil {
		_ = db.Close()
		t.Fatalf("reset database: %v", err)
	}

	cleanup := func() {
		_ = resetDatabase(ctx, db)
		_ = db.Close()
	}
	return store, cleanup
}

func resetDatabase(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `
SELECT tablename
FROM pg_tables
WHERE schemaname = 'public'
  AND tablename <> 'schema_migrations'
`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, quoteIdentifier(name))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}

	_, err = db.ExecContext(ctx, fmt.Sprintf("TRUNCATE %s CASCADE", strings.Join(tables, ", ")))
	return err
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
