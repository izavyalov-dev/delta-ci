package loadtest_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/internal/loadtest"
	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func testService(t *testing.T) (*orchestrator.Service, *state.Store) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}

	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store))
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, nil, nil)
	return service, store
}

func TestSustainedLoad(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()

	runCount := 100
	jobsPerRun := 5

	runIDs, err := loadtest.GenerateRuns(ctx, service, runCount, jobsPerRun)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("created %d runs", len(runIDs))

	workers := 4
	var completed int64
	var mu sync.Mutex
	var wg sync.WaitGroup

	sem := make(chan struct{}, workers)
	maxIter := runCount * jobsPerRun * 3
	emptyCount := 0

	for i := 0; i < maxIter; i++ {
		attemptID, err := service.DequeueJobAttempt(ctx, 30*time.Second)
		if err != nil {
			if errors.Is(err, state.ErrQueueEmpty) {
				emptyCount++
				if emptyCount > 30 {
					break
				}
				time.Sleep(200 * time.Millisecond)
				continue
			}
			t.Fatal(err)
		}
		emptyCount = 0

		sem <- struct{}{}
		wg.Add(1)
		go func(aid string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := loadtest.SimulateJobSuccess(ctx, service, store, 30*time.Second); err != nil {
				// SimulateJobSuccess dequeues again; for this test we grant+complete inline.
			}

			lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
				AttemptID:        aid,
				RunnerID:         "stress-worker",
				TTLSeconds:       120,
				HeartbeatSeconds: 30,
			})
			if err != nil {
				t.Logf("grant failed for %s: %v", aid, err)
				return
			}

			_ = service.AckLease(ctx, protocol.AckLease{
				Type:     "AckLease",
				LeaseID:  lease.LeaseID,
				RunnerID: "stress-worker",
			})

			err = service.CompleteLease(ctx, protocol.Complete{
				Type:       "Complete",
				LeaseID:    lease.LeaseID,
				Status:     protocol.CompleteStatusSucceeded,
				ExitCode:   0,
				FinishedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Logf("complete failed for %s: %v", aid, err)
				return
			}

			mu.Lock()
			completed++
			mu.Unlock()
		}(attemptID)
	}

	wg.Wait()
	t.Logf("completed %d jobs", completed)

	if completed == 0 {
		t.Fatal("no jobs were completed")
	}
}

func TestBackpressure(t *testing.T) {
	service, store := testService(t)
	ctx := context.Background()

	// Create runs to flood the queue.
	runIDs, err := loadtest.FloodQueue(ctx, service, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("flooded queue with %d runs", len(runIDs))

	// Dequeue one item and set its delivery_count to exceed max.
	attemptID, err := service.DequeueJobAttempt(ctx, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if err := loadtest.SetDeliveryCount(ctx, store, attemptID, 10); err != nil {
		t.Fatal(err)
	}

	// Run dead letter sweep.
	count, err := store.DeadLetterExpired(ctx, 5, 50)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dead-lettered %d items", count)

	if count == 0 {
		t.Fatal("expected at least one dead-lettered item")
	}
}

func TestGracefulShutdown(t *testing.T) {
	service, store := testService(t)
	ctx, cancel := context.WithCancel(context.Background())

	// Create a few runs.
	_, err := loadtest.GenerateRuns(ctx, service, 5, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Start a "worker" that processes one job then receives cancellation.
	var wg sync.WaitGroup
	var completedJobs int
	var mu sync.Mutex

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			attemptID, err := service.DequeueJobAttempt(ctx, 30*time.Second)
			if err != nil {
				if errors.Is(err, state.ErrQueueEmpty) || ctx.Err() != nil {
					return
				}
				t.Logf("dequeue error: %v", err)
				return
			}

			lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
				AttemptID:        attemptID,
				RunnerID:         "shutdown-worker",
				TTLSeconds:       120,
				HeartbeatSeconds: 30,
			})
			if err != nil {
				t.Logf("grant error: %v", err)
				continue
			}

			_ = service.AckLease(ctx, protocol.AckLease{
				Type:     "AckLease",
				LeaseID:  lease.LeaseID,
				RunnerID: "shutdown-worker",
			})

			// Simulate some work.
			time.Sleep(10 * time.Millisecond)

			_ = service.CompleteLease(context.Background(), protocol.Complete{
				Type:       "Complete",
				LeaseID:    lease.LeaseID,
				Status:     protocol.CompleteStatusSucceeded,
				ExitCode:   0,
				FinishedAt: time.Now().UTC(),
			})

			mu.Lock()
			completedJobs++
			mu.Unlock()

			// Cancel after first job to simulate SIGTERM.
			cancel()
		}
	}()

	wg.Wait()

	mu.Lock()
	t.Logf("completed %d jobs before shutdown", completedJobs)
	mu.Unlock()

	if completedJobs == 0 {
		t.Fatal("expected at least one job completed before shutdown")
	}

	_ = store
}
