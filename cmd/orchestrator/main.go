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
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/internal/vcs/github"
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
	case "worker":
		if err := runWorker(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "worker failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: orchestrator <serve|dogfood|worker> [flags]")
}

type aiSettings struct {
	Enabled        bool
	Provider       string
	Model          string
	Endpoint       string
	Token          string
	PromptVersion  string
	Timeout        time.Duration
	MaxOutputLen   int
	MaxCacheEvents int
	MaxFailures    int
	Cooldown       time.Duration
}

func addAIFlags(flags *flag.FlagSet) *aiSettings {
	settings := &aiSettings{
		Enabled:        envBool("DELTA_AI_ENABLED"),
		Provider:       os.Getenv("DELTA_AI_PROVIDER"),
		Model:          os.Getenv("DELTA_AI_MODEL"),
		Endpoint:       os.Getenv("DELTA_AI_ENDPOINT"),
		Token:          os.Getenv("DELTA_AI_TOKEN"),
		PromptVersion:  os.Getenv("DELTA_AI_PROMPT_VERSION"),
		Timeout:        envDuration("DELTA_AI_TIMEOUT"),
		MaxOutputLen:   envInt("DELTA_AI_MAX_OUTPUT_LEN"),
		MaxCacheEvents: envInt("DELTA_AI_MAX_CACHE_EVENTS"),
		MaxFailures:    envInt("DELTA_AI_CIRCUIT_FAILURES"),
		Cooldown:       envDuration("DELTA_AI_CIRCUIT_COOLDOWN"),
	}

	flags.BoolVar(&settings.Enabled, "ai-enabled", settings.Enabled, "Enable AI failure explanations")
	flags.StringVar(&settings.Provider, "ai-provider", settings.Provider, "AI provider name")
	flags.StringVar(&settings.Model, "ai-model", settings.Model, "AI model identifier")
	flags.StringVar(&settings.Endpoint, "ai-endpoint", settings.Endpoint, "AI HTTP endpoint for explanations")
	flags.StringVar(&settings.Token, "ai-token", settings.Token, "AI API token for Authorization header")
	flags.StringVar(&settings.PromptVersion, "ai-prompt-version", settings.PromptVersion, "AI prompt version")
	flags.DurationVar(&settings.Timeout, "ai-timeout", settings.Timeout, "AI request timeout (e.g., 3s)")
	flags.IntVar(&settings.MaxOutputLen, "ai-max-output-len", settings.MaxOutputLen, "Max AI output length")
	flags.IntVar(&settings.MaxCacheEvents, "ai-max-cache-events", settings.MaxCacheEvents, "Max cache events included in AI prompt")
	flags.IntVar(&settings.MaxFailures, "ai-circuit-failures", settings.MaxFailures, "AI circuit breaker failure threshold")
	flags.DurationVar(&settings.Cooldown, "ai-circuit-cooldown", settings.Cooldown, "AI circuit breaker cooldown")
	return settings
}

func buildFailureAnalyzer(store *state.Store, settings *aiSettings) (orchestrator.FailureAnalyzer, error) {
	analyzer := orchestrator.NewRuleBasedFailureAnalyzer()
	if settings == nil || !settings.Enabled {
		return analyzer, nil
	}
	if strings.TrimSpace(settings.Endpoint) == "" {
		return nil, errors.New("ai-enabled requires ai-endpoint")
	}

	client := &orchestrator.HTTPAIClient{
		Endpoint: settings.Endpoint,
		Token:    settings.Token,
	}
	explainer := orchestrator.NewAIExplainer(client, store, orchestrator.AIConfig{
		Enabled:        settings.Enabled,
		Provider:       settings.Provider,
		Model:          settings.Model,
		PromptVersion:  settings.PromptVersion,
		Timeout:        settings.Timeout,
		MaxOutputLen:   settings.MaxOutputLen,
		MaxCacheEvents: settings.MaxCacheEvents,
		MaxFailures:    settings.MaxFailures,
		Cooldown:       settings.Cooldown,
	})
	analyzer.Advisor = explainer
	analyzer.EnableAI = true
	return analyzer, nil
}

func envBool(name string) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return value
}

func envInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func envDuration(name string) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return value
}

func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ExitOnError)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres DSN")
	listen := flags.String("listen", ":8080", "Listen address")
	githubWebhookSecret := flags.String("github-webhook-secret", os.Getenv("GITHUB_WEBHOOK_SECRET"), "GitHub webhook secret")
	githubToken := flags.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub API token")
	githubAppID := flags.String("github-app-id", os.Getenv("GITHUB_APP_ID"), "GitHub App ID")
	githubAppInstallationID := flags.String("github-app-installation-id", os.Getenv("GITHUB_APP_INSTALLATION_ID"), "GitHub App installation ID")
	githubAppPrivateKey := flags.String("github-app-private-key", os.Getenv("GITHUB_APP_PRIVATE_KEY"), "GitHub App private key PEM")
	githubAppPrivateKeyFile := flags.String("github-app-private-key-file", os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"), "GitHub App private key PEM file")
	githubAPIURL := flags.String("github-api-url", os.Getenv("GITHUB_API_URL"), "GitHub API base URL")
	githubCheckName := flags.String("github-check-name", os.Getenv("GITHUB_CHECK_NAME"), "GitHub check run name")
	aiSettings := addAIFlags(flags)
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

	reporter, err := buildGitHubReporter(store, *githubToken, *githubAppID, *githubAppInstallationID, *githubAppPrivateKey, *githubAppPrivateKeyFile, *githubAPIURL, *githubCheckName)
	if err != nil {
		return err
	}
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store))
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, reporter, analyzer)
	handler := orchestrator.NewHTTPHandler(service, observability.NewLogger("orchestrator.http"), orchestrator.HTTPConfig{
		GitHubWebhookSecret: *githubWebhookSecret,
	})

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
	githubToken := flags.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub API token")
	githubAppID := flags.String("github-app-id", os.Getenv("GITHUB_APP_ID"), "GitHub App ID")
	githubAppInstallationID := flags.String("github-app-installation-id", os.Getenv("GITHUB_APP_INSTALLATION_ID"), "GitHub App installation ID")
	githubAppPrivateKey := flags.String("github-app-private-key", os.Getenv("GITHUB_APP_PRIVATE_KEY"), "GitHub App private key PEM")
	githubAppPrivateKeyFile := flags.String("github-app-private-key-file", os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE"), "GitHub App private key PEM file")
	githubAPIURL := flags.String("github-api-url", os.Getenv("GITHUB_API_URL"), "GitHub API base URL")
	githubCheckName := flags.String("github-check-name", os.Getenv("GITHUB_CHECK_NAME"), "GitHub check run name")
	aiSettings := addAIFlags(flags)
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

	reporter, err := buildGitHubReporter(store, *githubToken, *githubAppID, *githubAppInstallationID, *githubAppPrivateKey, *githubAppPrivateKeyFile, *githubAPIURL, *githubCheckName)
	if err != nil {
		return err
	}
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store))
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, reporter, analyzer)
	handler := orchestrator.NewHTTPHandler(service, observability.NewLogger("orchestrator.http"), orchestrator.HTTPConfig{})

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

func buildGitHubReporter(store *state.Store, token, appID, appInstallationID, appPrivateKey, appPrivateKeyFile, apiURL, checkName string) (orchestrator.StatusReporter, error) {
	if appID != "" || appInstallationID != "" || appPrivateKey != "" || appPrivateKeyFile != "" {
		if appID == "" || appInstallationID == "" {
			return nil, errors.New("github app id and installation id required")
		}
		key, err := loadGitHubAppKey(appPrivateKey, appPrivateKeyFile)
		if err != nil {
			return nil, err
		}
		client := github.NewAppClient(nil)
		if apiURL != "" {
			client.BaseURL = apiURL
		}
		provider, err := github.NewAppTokenProvider(appID, appInstallationID, key, client.BaseURL)
		if err != nil {
			return nil, err
		}
		client.TokenProvider = provider
		return github.NewReporter(store, client, observability.NewLogger("status.github"), checkName), nil
	}

	if token == "" {
		return orchestrator.NoopStatusReporter{}, nil
	}
	client := github.NewClient(token)
	if apiURL != "" {
		client.BaseURL = apiURL
	}
	return github.NewReporter(store, client, observability.NewLogger("status.github"), checkName), nil
}

func loadGitHubAppKey(rawKey, keyFile string) ([]byte, error) {
	if keyFile != "" {
		return os.ReadFile(keyFile)
	}
	if rawKey == "" {
		return nil, errors.New("github app private key required")
	}
	rawKey = strings.ReplaceAll(rawKey, "\\n", "\n")
	return []byte(rawKey), nil
}

func runWorker(args []string) error {
	flags := flag.NewFlagSet("worker", flag.ExitOnError)
	databaseURL := flags.String("database-url", os.Getenv("DATABASE_URL"), "Postgres DSN")
	orchestratorURL := flags.String("orchestrator-url", "http://localhost:8080", "Orchestrator base URL")
	runnerID := flags.String("runner-id", "local-worker", "Runner ID for leases")
	workdir := flags.String("workdir", ".", "Working directory for runner execution")
	runnerCmd := flags.String("runner-cmd", "go run ./runner", "Command used to launch the runner")
	logDir := flags.String("runner-log-dir", ".delta-ci/logs", "Directory for runner logs")
	s3Bucket := flags.String("s3-bucket", "", "S3 bucket for log uploads")
	s3Prefix := flags.String("s3-prefix", "", "S3 key prefix for log uploads")
	s3Region := flags.String("s3-region", "", "S3 region for log uploads")
	visibilityTimeout := flags.Duration("visibility-timeout", 30*time.Second, "Queue visibility timeout")
	pollInterval := flags.Duration("poll-interval", 2*time.Second, "Delay between empty queue polls")
	continueOnRunnerError := flags.Bool("continue-on-runner-error", true, "Keep worker running after a runner error")
	aiSettings := addAIFlags(flags)
	_ = flags.Parse(args)

	if *databaseURL == "" {
		return errors.New("database-url or DATABASE_URL required")
	}
	if *orchestratorURL == "" {
		return errors.New("orchestrator-url required")
	}
	if *runnerID == "" {
		return errors.New("runner-id required")
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

	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store))
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, nil, analyzer)
	logger := observability.NewLogger("worker")

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		return err
	}

	leaseDir, err := os.MkdirTemp("", "delta-ci-worker-leases")
	if err != nil {
		return err
	}

	logger.Info("worker started", "event", "worker_started", "orchestrator_url", *orchestratorURL)

	for {
		attemptID, err := service.DequeueJobAttempt(ctx, *visibilityTimeout)
		if err != nil {
			if errors.Is(err, state.ErrQueueEmpty) {
				time.Sleep(*pollInterval)
				continue
			}
			return err
		}

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
		if err := runRunner(ctx, *runnerCmd, *orchestratorURL, *runnerID, leasePath, *workdir, logPath, *s3Bucket, *s3Prefix, *s3Region); err != nil {
			logger.Warn("runner exited with error", "event", "runner_failed", "error", err)
			if *continueOnRunnerError {
				continue
			}
			return err
		}
	}
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
