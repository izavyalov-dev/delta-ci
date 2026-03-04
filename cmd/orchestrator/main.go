package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/izavyalov-dev/delta-ci/internal/notify"
	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/internal/vcs/github"
	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
	"github.com/izavyalov-dev/delta-ci/web"
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
	case "trigger":
		if err := runTrigger(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "trigger failed: %v\n", err)
			os.Exit(1)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: orchestrator <serve|dogfood|worker|trigger> [flags]")
}

// stringSliceFlag implements flag.Value for repeatable string flags.
type stringSliceFlag []string

func (f *stringSliceFlag) String() string { return strings.Join(*f, ",") }
func (f *stringSliceFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func buildPluginRegistry(extraPaths []string) *planner.PluginRegistry {
	plugins := []planner.LanguagePlugin{planner.GoLanguagePlugin{}}
	for _, path := range extraPaths {
		name := filepath.Base(path)
		name = strings.TrimPrefix(name, "delta-ci-lang-")
		if name == "" || name == path {
			name = filepath.Base(path)
		}
		plugins = append(plugins, &planner.ExternalLanguagePlugin{
			PluginName: name,
			Path:       path,
		})
	}
	for _, ep := range planner.DiscoverExternalPlugins() {
		plugins = append(plugins, ep)
	}
	return planner.NewPluginRegistry(plugins...)
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

type notifySettings struct {
	WebhookURL    string
	WebhookSecret string
	WebhookEvents string
	SlackURL      string
	SlackEvents   string
	DashboardURL  string
}

func addNotifyFlags(flags *flag.FlagSet) *notifySettings {
	s := &notifySettings{
		WebhookURL:    os.Getenv("DELTA_NOTIFY_WEBHOOK_URL"),
		WebhookSecret: os.Getenv("DELTA_NOTIFY_WEBHOOK_SECRET"),
		WebhookEvents: os.Getenv("DELTA_NOTIFY_WEBHOOK_EVENTS"),
		SlackURL:      os.Getenv("DELTA_NOTIFY_SLACK_WEBHOOK_URL"),
		SlackEvents:   os.Getenv("DELTA_NOTIFY_SLACK_EVENTS"),
		DashboardURL:  os.Getenv("DELTA_DASHBOARD_URL"),
	}
	flags.StringVar(&s.WebhookURL, "notify-webhook-url", s.WebhookURL, "Webhook URL for run notifications")
	flags.StringVar(&s.WebhookSecret, "notify-webhook-secret", s.WebhookSecret, "HMAC-SHA256 signing secret for webhook")
	flags.StringVar(&s.WebhookEvents, "notify-webhook-events", s.WebhookEvents, "Webhook event filter: all or terminal-only (default)")
	flags.StringVar(&s.SlackURL, "notify-slack-webhook-url", s.SlackURL, "Slack incoming webhook URL")
	flags.StringVar(&s.SlackEvents, "notify-slack-events", s.SlackEvents, "Slack event filter: all or terminal-only (default)")
	flags.StringVar(&s.DashboardURL, "notify-dashboard-url", s.DashboardURL, "Dashboard base URL included in notifications")
	return s
}

func buildReporter(store *state.Store, ghReporter orchestrator.StatusReporter, ns *notifySettings) orchestrator.StatusReporter {
	reporters := []orchestrator.StatusReporter{ghReporter}

	if ns.WebhookURL != "" {
		reporters = append(reporters, notify.NewWebhookReporter(store, notify.WebhookConfig{
			URL:    ns.WebhookURL,
			Secret: ns.WebhookSecret,
			Events: notify.EventFilter(ns.WebhookEvents),
		}, observability.NewLogger("notify.webhook"), ns.DashboardURL))
	}

	if ns.SlackURL != "" {
		reporters = append(reporters, notify.NewSlackReporter(store, notify.SlackConfig{
			WebhookURL: ns.SlackURL,
			Events:     notify.EventFilter(ns.SlackEvents),
		}, observability.NewLogger("notify.slack"), ns.DashboardURL))
	}

	multi := notify.NewMultiReporter(reporters...)
	if multi.Len() == 0 {
		return orchestrator.NoopStatusReporter{}
	}
	if multi.Len() == 1 {
		return ghReporter
	}
	return multi
}

func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
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
	gitBaseURL := flags.String("git-base-url", envOrDefault("DELTA_GIT_BASE_URL", "https://github.com"), "Base URL for git clone (e.g. https://github.com)")
	webEnabled := flags.Bool("web-enabled", !envBool("DELTA_WEB_DISABLED"), "Enable web dashboard")
	webDev := flags.Bool("web-dev", envBool("DELTA_WEB_DEV"), "Serve templates from disk for hot-reload")
	pprofEnabled := flags.Bool("pprof-enabled", false, "Enable pprof profiling endpoints")
	pprofListen := flags.String("pprof-listen", ":6060", "Listen address for pprof server")
	dbMaxOpenConns := flags.Int("db-max-open-conns", 10, "Maximum open database connections")
	dbMaxIdleConns := flags.Int("db-max-idle-conns", 5, "Maximum idle database connections")
	dbConnMaxLifetime := flags.Duration("db-conn-max-lifetime", 30*time.Minute, "Maximum connection lifetime")
	var langPlugins stringSliceFlag
	flags.Var(&langPlugins, "language-plugin", "Path to external language plugin (repeatable)")
	notifySettings := addNotifyFlags(flags)
	aiSettings := addAIFlags(flags)
	_ = flags.Parse(args)

	if *databaseURL == "" {
		return errors.New("database-url or DATABASE_URL required")
	}

	ctx := context.Background()
	db, err := openDBWithConfig(ctx, *databaseURL, dbPoolConfig{
		MaxOpenConns:    *dbMaxOpenConns,
		MaxIdleConns:    *dbMaxIdleConns,
		ConnMaxLifetime: *dbConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		return err
	}

	logger := observability.NewLogger("orchestrator")

	if *pprofEnabled {
		startPprofServer(*pprofListen, logger)
	}

	ghReporter, err := buildGitHubReporter(store, *githubToken, *githubAppID, *githubAppInstallationID, *githubAppPrivateKey, *githubAppPrivateKeyFile, *githubAPIURL, *githubCheckName)
	if err != nil {
		return err
	}
	reporter := buildReporter(store, ghReporter, notifySettings)
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	registry := buildPluginRegistry(langPlugins)
	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store), registry)
	service := orchestrator.NewService(store, plan, orchestrator.NewQueueDispatcher(store), nil, reporter, analyzer)
	service.WithGitConfig(*gitBaseURL, *githubToken)
	apiHandler := orchestrator.NewHTTPHandler(service, observability.NewLogger("orchestrator.http"), orchestrator.HTTPConfig{
		GitHubWebhookSecret: *githubWebhookSecret,
	})

	var rootHandler http.Handler
	if *webEnabled {
		webHandler := web.NewHandler(service, observability.NewLogger("web"), web.Config{
			DevMode: *webDev,
		})
		mux := http.NewServeMux()
		mux.Handle("/api/", apiHandler)
		mux.Handle("/healthz", apiHandler)
		mux.Handle("/metrics", apiHandler)
		mux.Handle("/", webHandler)
		rootHandler = mux
	} else {
		rootHandler = apiHandler
	}

	server := &http.Server{
		Addr:              *listen,
		Handler:           rootHandler,
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
	var dogfoodLangPlugins stringSliceFlag
	flags.Var(&dogfoodLangPlugins, "language-plugin", "Path to external language plugin (repeatable)")
	notifySettings := addNotifyFlags(flags)
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

	ghReporter, err := buildGitHubReporter(store, *githubToken, *githubAppID, *githubAppInstallationID, *githubAppPrivateKey, *githubAppPrivateKeyFile, *githubAPIURL, *githubCheckName)
	if err != nil {
		return err
	}
	reporter := buildReporter(store, ghReporter, notifySettings)
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	dogfoodRegistry := buildPluginRegistry(dogfoodLangPlugins)
	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store), dogfoodRegistry)
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

func runTrigger(args []string) error {
	flags := flag.NewFlagSet("trigger", flag.ExitOnError)
	orchestratorURL := flags.String("orchestrator-url", "http://localhost:8080", "Orchestrator base URL")
	repoID := flags.String("repo-id", "", "Repository ID (auto-detected from git remote if not set)")
	ref := flags.String("ref", "", "Git ref (auto-detected from current branch if not set)")
	commitSHA := flags.String("commit-sha", "", "Commit SHA (auto-detected from git HEAD if not set)")
	_ = flags.Parse(args)

	if *repoID == "" {
		*repoID = triggerAutoDetectRepoID()
	}
	if *ref == "" {
		branch := gitAutoDetect("symbolic-ref", "--short", "HEAD")
		if branch != "" {
			if !strings.Contains(branch, "/") {
				branch = "refs/heads/" + branch
			}
			*ref = branch
		}
	}
	if *commitSHA == "" {
		*commitSHA = gitAutoDetect("rev-parse", "HEAD")
	}

	if *repoID == "" {
		return errors.New("repo-id required (could not auto-detect from git remote)")
	}
	if *ref == "" {
		return errors.New("ref required (could not auto-detect from git branch)")
	}
	if *commitSHA == "" {
		return errors.New("commit-sha required (could not auto-detect from git HEAD)")
	}

	payload, err := json.Marshal(map[string]string{
		"repo_id":    *repoID,
		"ref":        *ref,
		"commit_sha": *commitSHA,
	})
	if err != nil {
		return err
	}

	baseURL := strings.TrimRight(*orchestratorURL, "/")
	resp, err := http.Post(baseURL+"/api/v1/runs", "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("POST %s/api/v1/runs: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		RunID string `json:"run_id"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	fmt.Printf("run_id: %s\n", result.RunID)
	fmt.Printf("state:  %s\n", result.State)
	fmt.Printf("url:    %s/runs/%s\n", baseURL, result.RunID)
	return nil
}

// gitAutoDetect runs a git command and returns trimmed stdout, or "" on error.
func gitAutoDetect(gitArgs ...string) string {
	out, err := exec.Command("git", gitArgs...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// triggerAutoDetectRepoID infers a repo ID from the git remote URL.
// It takes the last path segment and strips the ".git" suffix.
func triggerAutoDetectRepoID() string {
	remote := gitAutoDetect("remote", "get-url", "origin")
	if remote == "" {
		return ""
	}
	base := filepath.Base(remote)
	return strings.TrimSuffix(base, ".git")
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
	pollInterval := flags.Duration("poll-interval", 2*time.Second, "Base delay between empty queue polls")
	maxPollInterval := flags.Duration("max-poll-interval", 30*time.Second, "Maximum poll interval for adaptive backoff")
	continueOnRunnerError := flags.Bool("continue-on-runner-error", true, "Keep worker running after a runner error")
	maxConcurrency := flags.Int("max-concurrency", 4, "Maximum number of concurrent jobs")
	shutdownTimeout := flags.Duration("shutdown-timeout", 60*time.Second, "Maximum wait for in-flight jobs on shutdown")
	maxDeliveryCount := flags.Int("max-delivery-count", 5, "Maximum delivery attempts before dead-lettering")
	pprofEnabled := flags.Bool("pprof-enabled", false, "Enable pprof profiling endpoints")
	pprofListen := flags.String("pprof-listen", ":6060", "Listen address for pprof server")
	dbMaxOpenConns := flags.Int("db-max-open-conns", 10, "Maximum open database connections")
	dbMaxIdleConns := flags.Int("db-max-idle-conns", 5, "Maximum idle database connections")
	dbConnMaxLifetime := flags.Duration("db-conn-max-lifetime", 30*time.Minute, "Maximum connection lifetime")
	var workerLangPlugins stringSliceFlag
	flags.Var(&workerLangPlugins, "language-plugin", "Path to external language plugin (repeatable)")
	notifySettings := addNotifyFlags(flags)
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
	if *maxConcurrency < 1 {
		return errors.New("max-concurrency must be >= 1")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := openDBWithConfig(ctx, *databaseURL, dbPoolConfig{
		MaxOpenConns:    *dbMaxOpenConns,
		MaxIdleConns:    *dbMaxIdleConns,
		ConnMaxLifetime: *dbConnMaxLifetime,
	})
	if err != nil {
		return err
	}
	defer db.Close()

	store := state.NewStore(db)
	if err := store.ApplyMigrations(ctx); err != nil {
		return err
	}

	metrics := observability.NewMetrics(nil)

	reporter := buildReporter(store, nil, notifySettings)
	workerRegistry := buildPluginRegistry(workerLangPlugins)
	plan := planner.NewDiffPlanner("", planner.StaticPlanner{}, orchestrator.NewRecipeStore(store), workerRegistry)
	analyzer, err := buildFailureAnalyzer(store, aiSettings)
	if err != nil {
		return err
	}
	service := orchestrator.NewServiceWithMetrics(store, plan, orchestrator.NewQueueDispatcher(store), nil, reporter, analyzer, metrics)
	logger := observability.NewLogger("worker")

	if err := os.MkdirAll(*logDir, 0o755); err != nil {
		return err
	}

	leaseDir, err := os.MkdirTemp("", "delta-ci-worker-leases")
	if err != nil {
		return err
	}

	if *pprofEnabled {
		startPprofServer(*pprofListen, logger)
	}

	// Start dead letter sweeper.
	dlStop := startDeadLetterSweeper(store, logger, 30*time.Second, *maxDeliveryCount)
	defer close(dlStop)

	// Start gauge collector for queue depth and active leases.
	gaugeStop := startGaugeCollector(store, metrics, logger, 15*time.Second)
	defer close(gaugeStop)

	logger.Info("worker started",
		"event", "worker_started",
		"orchestrator_url", *orchestratorURL,
		"max_concurrency", *maxConcurrency,
		"max_delivery_count", *maxDeliveryCount,
	)

	sem := make(chan struct{}, *maxConcurrency)
	var wg sync.WaitGroup
	consecutiveEmpty := 0

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutdown signal received, waiting for in-flight jobs", "event", "worker_shutting_down")
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()
			select {
			case <-done:
				logger.Info("all in-flight jobs completed", "event", "worker_shutdown_clean")
			case <-time.After(*shutdownTimeout):
				logger.Warn("shutdown timeout exceeded, exiting", "event", "worker_shutdown_timeout")
			}
			return nil
		case sem <- struct{}{}:
			// Acquired a concurrency slot.
		}

		attemptID, err := service.DequeueJobAttempt(ctx, *visibilityTimeout)
		if err != nil {
			<-sem // release slot
			if errors.Is(err, state.ErrQueueEmpty) {
				consecutiveEmpty++
				backoff := adaptivePollInterval(*pollInterval, *maxPollInterval, consecutiveEmpty)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
				}
				continue
			}
			if ctx.Err() != nil {
				continue // will hit shutdown path above
			}
			return err
		}
		consecutiveEmpty = 0
		metrics.SetWorkerActive(1)

		wg.Add(1)
		go func(attemptID string) {
			defer wg.Done()
			defer func() { <-sem }()

			lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
				AttemptID:        attemptID,
				RunnerID:         *runnerID,
				TTLSeconds:       120,
				HeartbeatSeconds: 30,
			})
			if err != nil {
				logger.Error("grant lease failed", "event", "grant_lease_failed", "attempt_id", attemptID, "error", err)
				return
			}

			leasePath := filepath.Join(leaseDir, attemptID+".json")
			if err := writeLeaseFile(leasePath, lease); err != nil {
				logger.Error("write lease file failed", "event", "write_lease_failed", "attempt_id", attemptID, "error", err)
				return
			}

			logPath := filepath.Join(*logDir, attemptID+".log")
			if err := runRunner(ctx, *runnerCmd, *orchestratorURL, *runnerID, leasePath, *workdir, logPath, *s3Bucket, *s3Prefix, *s3Region); err != nil {
				logger.Warn("runner exited with error", "event", "runner_failed", "attempt_id", attemptID, "error", err)
				if !*continueOnRunnerError {
					stop() // trigger shutdown
				}
			}
		}(attemptID)
	}
}

type dbPoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func openDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	return openDBWithConfig(ctx, databaseURL, dbPoolConfig{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 30 * time.Minute,
	})
}

func openDBWithConfig(ctx context.Context, databaseURL string, cfg dbPoolConfig) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	} else {
		db.SetMaxOpenConns(10)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	} else {
		db.SetMaxIdleConns(5)
	}

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

func adaptivePollInterval(base, max time.Duration, consecutiveEmpty int) time.Duration {
	if consecutiveEmpty <= 1 {
		return base
	}
	multiplier := math.Pow(2, float64(consecutiveEmpty-1))
	interval := time.Duration(float64(base) * multiplier)
	if interval > max {
		return max
	}
	return interval
}

func startPprofServer(listen string, logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		logger.Info("pprof server started", "event", "pprof_started", "listen", listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("pprof server failed", "event", "pprof_failed", "error", err)
		}
	}()
}

func startDeadLetterSweeper(store *state.Store, logger *slog.Logger, interval time.Duration, maxDeliveries int) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				count, err := store.DeadLetterExpired(context.Background(), maxDeliveries, 50)
				if err != nil {
					logger.Error("dead letter sweep failed", "event", "dead_letter_sweep_failed", "error", err)
				} else if count > 0 {
					logger.Info("dead letter sweep completed", "event", "dead_letter_sweep_completed", "count", count)
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}

func startGaugeCollector(store *state.Store, metrics *observability.Metrics, logger *slog.Logger, interval time.Duration) chan struct{} {
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				ctx := context.Background()
				depth, err := store.QueueDepth(ctx)
				if err != nil {
					logger.Error("queue depth query failed", "event", "gauge_query_failed", "error", err)
				} else {
					metrics.SetQueueDepth(float64(depth))
				}
				active, err := store.ActiveLeaseCount(ctx)
				if err != nil {
					logger.Error("active lease count query failed", "event", "gauge_query_failed", "error", err)
				} else {
					metrics.SetActiveLeases(float64(active))
				}
			case <-stop:
				return
			}
		}
	}()
	return stop
}
