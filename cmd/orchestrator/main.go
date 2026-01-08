package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "serve failed: %v\n", err)
			os.Exit(1)
		}
	case "dogfood":
		if err := runDogfood(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "dogfood failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: orchestrator <serve|dogfood> [flags]")
}

func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres DSN")
	listen := flags.String("listen", ":8080", "Listen address")
	_ = flags.Parse(args)

	if *databaseURL == "" {
		return errors.New("database-url or DATABASE_URL required")
	}

	ctx := context.Background()
	db, err := openDB(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		return err
	}

	service := orchestrator.NewService(store, planner.StaticPlanner{}, orchestrator.NewQueueDispatcher(store), nil)
	handler := orchestrator.NewHTTPHandler(service, observability.NewLogger("orchestrator.http"))

	server := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	stop := startLeaseSweeper(service, observability.NewLogger("orchestrator.sweeper"), 5*time.Second)
	defer close(stop)

	return server.ListenAndServe()
}

func runDogfood(args []string) error {
	flags := flag.NewFlagSet("dogfood", flag.ExitOnError)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres DSN")
	listen := flags.String("listen", ":8080", "Listen address for runner callbacks")
	repoID := flags.String("repo-id", "delta-ci", "Repository ID")
	ref := flags.String("ref", "refs/heads/phase_0", "Git ref")
	commitSHA := flags.String("commit-sha", "local", "Commit SHA placeholder")
	runnerID := flags.String("runner-id", "dogfood-runner", "Runner ID for the dogfood run")
	workdir := flags.String("workdir", ".", "Working directory for runner execution")
	runnerCmd := flags.String("runner-cmd", "go run ./runner", "Command used to launch the runner")
	logDir := flags.String("runner-log-dir", ".delta-ci/logs", "Directory for runner logs")
	s3Bucket := flags.String("s3-bucket", "", "S3 bucket for log uploads")
	s3Prefix := flags.String("s3-prefix", "", "S3 key prefix for log uploads")
	s3Region := flags.String("s3-region", "", "S3 region for log uploads")
	visibilityTimeout := flags.Duration("visibility-timeout", 30*time.Second, "Queue visibility timeout")
	continueOnRunnerError := flags.Bool("continue-on-runner-error", false, "Keep the dogfood loop running after a runner error")
	_ = flags.Parse(args)

	if *databaseURL == "" {
		return errors.New("database-url or DATABASE_URL required")
	}

	ctx := context.Background()
	db, err := openDB(ctx, *databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		return err
	}

	service := orchestrator.NewService(store, planner.StaticPlanner{}, orchestrator.NewQueueDispatcher(store), nil)
	handler := orchestrator.NewHTTPHandler(service, observability.NewLogger("orchestrator.http"))

	server, baseURL, err := startServer(handler, *listen)
	if err != nil {
		return err
	}
	defer server.Shutdown(ctx)

	logger := observability.NewLogger("dogfood")
	logger.Info("server started", "event", "server_started", "url", baseURL)

	stop := startLeaseSweeper(service, observability.NewLogger("orchestrator.sweeper"), 5*time.Second)
	defer close(stop)

	runDetails, err := service.CreateRun(ctx, orchestrator.CreateRunRequest{
		RepoID:    *repoID,
		Ref:       *ref,
		CommitSHA: *commitSHA,
	})
	if err != nil {
		return err
	}
	logger.Info("run created", "event", "run_created", "run_id", runDetails.Run.ID)

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		return err
	}

	leaseDir, err := os.MkdirTemp("", "delta-ci-leases")
	if err != nil {
		return err
	}

	idleTimeout := 2 * time.Minute
	lastActivity := time.Now()
	for {
		run, err := store.GetRun(ctx, runDetails.Run.ID)
		if err != nil {
			return err
		}
		if isTerminalRun(run.State) {
			break
		}

		attemptID, err := service.DequeueJobAttempt(ctx, *visibilityTimeout)
		if err != nil {
			if errors.Is(err, state.ErrQueueEmpty) {
				if time.Since(lastActivity) > idleTimeout {
					return errors.New("idle timeout waiting for queued attempts")
				}
				time.Sleep(2 * time.Second)
				continue
			}
			return err
		}
		lastActivity = time.Now()

		lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
			AttemptID:        attemptID,
			RunnerID:         *runnerID,
			TTLSeconds:       120,
			HeartbeatSeconds: 30,
		})
		if err != nil {
			return err
		}

		leasePath := filepath.Join(leaseDir, attemptID+".json")
		if err := writeLeaseFile(leasePath, lease); err != nil {
			return err
		}

		logPath := filepath.Join(*logDir, attemptID+".log")
		if err := runRunner(ctx, *runnerCmd, baseURL, *runnerID, leasePath, *workdir, logPath, *s3Bucket, *s3Prefix, *s3Region); err != nil {
			logger.Warn("runner exited with error", "event", "runner_failed", "error", err)
			if *continueOnRunnerError {
				continue
			}
			return err
		}
	}

	logger.Info("dogfood run finished", "event", "run_finished", "run_id", runDetails.Run.ID)
	return nil
}

func openDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func startServer(handler http.Handler, listen string) (*http.Server, string, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, "", err
	}

	addr := ln.Addr().(*net.TCPAddr)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", addr.Port)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		_ = server.Serve(ln)
	}()

	return server, baseURL, nil
}

func startLeaseSweeper(service *orchestrator.Service, logger *slog.Logger, interval time.Duration) chan struct{} {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				count, err := service.ExpireLeases(context.Background(), 25)
				if err != nil && !errors.Is(err, state.ErrNoExpiredLeases) {
					logger.Error("lease sweep failed", "event", "lease_sweep_failed", "error", err)
				} else if count > 0 {
					logger.Info("lease sweep completed", "event", "lease_sweep_completed", "count", count)
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}

func writeLeaseFile(path string, lease protocol.LeaseGranted) error {
	data, err := json.MarshalIndent(lease, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func runRunner(ctx context.Context, runnerCmd, baseURL, runnerID, leasePath, workdir, logPath, s3Bucket, s3Prefix, s3Region string) error {
	parts := strings.Fields(runnerCmd)
	if len(parts) == 0 {
		return errors.New("runner-cmd is empty")
	}

	args := append(parts[1:], "-orchestrator", baseURL, "-runner-id", runnerID, "-lease", leasePath, "-workdir", workdir, "-log", logPath)
	if s3Bucket != "" {
		args = append(args, "-s3-bucket", s3Bucket)
	}
	if s3Prefix != "" {
		args = append(args, "-s3-prefix", s3Prefix)
	}
	if s3Region != "" {
		args = append(args, "-s3-region", s3Region)
	}

	cmd := exec.CommandContext(ctx, parts[0], args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func isTerminalRun(runState state.RunState) bool {
	switch runState {
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateCanceled, state.RunStateTimeout, state.RunStateReported:
		return true
	default:
		return false
	}
}
