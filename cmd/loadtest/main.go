package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func main() {
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "Postgres DSN")
	concurrency := flag.Int("concurrency", 4, "Number of concurrent workers")
	runs := flag.Int("runs", 50, "Number of runs to create")
	jobsPerRun := flag.Int("jobs-per-run", 3, "Number of jobs per run")
	visibilityTimeout := flag.Duration("visibility-timeout", 30*time.Second, "Queue visibility timeout")
	flag.Parse()

	if *databaseURL == "" {
		fmt.Fprintln(os.Stderr, "database-url or DATABASE_URL required")
		os.Exit(1)
	}

	ctx := context.Background()
	logger := observability.NewLogger("loadtest")

	db, err := sql.Open("pgx", *databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "ping db: %v\n", err)
		os.Exit(1)
	}

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "migrations: %v\n", err)
		os.Exit(1)
	}

	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store))
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, nil, nil)

	logger.Info("creating runs", "runs", *runs, "jobs_per_run", *jobsPerRun)
	start := time.Now()

	// Create all runs.
	var runIDs []string
	for i := 0; i < *runs; i++ {
		details, err := service.CreateRun(ctx, orchestrator.CreateRunRequest{
			RepoID:    fmt.Sprintf("loadtest-repo-%d", i%5),
			Ref:       "refs/heads/loadtest",
			CommitSHA: fmt.Sprintf("loadtest-sha-%d", i),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "create run %d: %v\n", i, err)
			os.Exit(1)
		}
		runIDs = append(runIDs, details.Run.ID)
	}
	logger.Info("runs created", "count", len(runIDs), "elapsed", time.Since(start).String())

	// Process all jobs with concurrent workers.
	var completed atomic.Int64
	var errCount atomic.Int64
	var latencies sync.Mutex
	var allLatencies []time.Duration

	var wg sync.WaitGroup
	sem := make(chan struct{}, *concurrency)

	workerStart := time.Now()
	maxIterations := *runs * *jobsPerRun * 3 // safety limit
	emptyCount := 0

	for i := 0; i < maxIterations; i++ {
		attemptID, err := service.DequeueJobAttempt(ctx, *visibilityTimeout)
		if err != nil {
			if errors.Is(err, state.ErrQueueEmpty) {
				emptyCount++
				if emptyCount > 20 {
					break
				}
				time.Sleep(500 * time.Millisecond)
				continue
			}
			logger.Error("dequeue failed", "error", err)
			errCount.Add(1)
			continue
		}
		emptyCount = 0

		sem <- struct{}{}
		wg.Add(1)
		go func(aid string) {
			defer wg.Done()
			defer func() { <-sem }()

			jobStart := time.Now()

			lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
				AttemptID:        aid,
				RunnerID:         "loadtest-worker",
				TTLSeconds:       120,
				HeartbeatSeconds: 30,
			})
			if err != nil {
				logger.Error("grant lease failed", "attempt_id", aid, "error", err)
				errCount.Add(1)
				return
			}

			// Simulate ack.
			if err := service.AckLease(ctx, protocol.AckLease{
				Type:     "AckLease",
				LeaseID:  lease.LeaseID,
				RunnerID: "loadtest-worker",
			}); err != nil {
				logger.Error("ack lease failed", "lease_id", lease.LeaseID, "error", err)
				errCount.Add(1)
				return
			}

			// Simulate completion.
			if err := service.CompleteLease(ctx, protocol.Complete{
				Type:       "Complete",
				LeaseID:    lease.LeaseID,
				Status:     protocol.CompleteStatusSucceeded,
				ExitCode:   0,
				FinishedAt: time.Now().UTC(),
			}); err != nil {
				logger.Error("complete lease failed", "lease_id", lease.LeaseID, "error", err)
				errCount.Add(1)
				return
			}

			elapsed := time.Since(jobStart)
			latencies.Lock()
			allLatencies = append(allLatencies, elapsed)
			latencies.Unlock()
			completed.Add(1)
		}(attemptID)
	}

	wg.Wait()
	totalElapsed := time.Since(workerStart)

	// Report results.
	total := completed.Load()
	errs := errCount.Load()
	throughput := float64(total) / totalElapsed.Seconds()

	logger.Info("load test complete",
		"completed", total,
		"errors", errs,
		"elapsed", totalElapsed.String(),
		"throughput_per_sec", fmt.Sprintf("%.2f", throughput),
	)

	if len(allLatencies) > 0 {
		sortDurations(allLatencies)
		p50 := allLatencies[len(allLatencies)*50/100]
		p95 := allLatencies[len(allLatencies)*95/100]
		p99idx := len(allLatencies) * 99 / 100
		if p99idx >= len(allLatencies) {
			p99idx = len(allLatencies) - 1
		}
		p99 := allLatencies[p99idx]

		logger.Info("latency percentiles",
			"p50", p50.String(),
			"p95", p95.String(),
			"p99", p99.String(),
		)
	}

	if errs > 0 {
		os.Exit(1)
	}
}

func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}
