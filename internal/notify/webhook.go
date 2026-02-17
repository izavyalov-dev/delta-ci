package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

// EventFilter controls which run state changes trigger a notification.
type EventFilter string

const (
	EventFilterAll          EventFilter = "all"
	EventFilterTerminalOnly EventFilter = "terminal-only"
)

// WebhookConfig holds configuration for the webhook reporter.
type WebhookConfig struct {
	URL    string
	Secret string      // HMAC-SHA256 signing secret (optional)
	Events EventFilter // "all" or "terminal-only" (default)
}

// WebhookPayload is the JSON body sent to the webhook endpoint.
type WebhookPayload struct {
	Event        string       `json:"event"`
	RunID        string       `json:"run_id"`
	RepoID       string       `json:"repo_id"`
	Ref          string       `json:"ref"`
	CommitSHA    string       `json:"commit_sha"`
	State        string       `json:"state"`
	Jobs         []JobSummary `json:"jobs"`
	DurationMS   int64        `json:"duration_ms"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	DashboardURL string       `json:"dashboard_url,omitempty"`
}

// JobSummary is the per-job portion of the webhook payload.
type JobSummary struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Required bool   `json:"required"`
}

// WebhookReporter sends an HTTP POST with run details on state changes.
type WebhookReporter struct {
	store        *state.Store
	client       *http.Client
	config       WebhookConfig
	logger       *slog.Logger
	dashboardURL string
}

// NewWebhookReporter creates a WebhookReporter.
func NewWebhookReporter(store *state.Store, config WebhookConfig, logger *slog.Logger, dashboardURL string) *WebhookReporter {
	if config.Events == "" {
		config.Events = EventFilterTerminalOnly
	}
	return &WebhookReporter{
		store:        store,
		client:       &http.Client{Timeout: 10 * time.Second},
		config:       config,
		logger:       logger,
		dashboardURL: dashboardURL,
	}
}

func (w *WebhookReporter) ReportRun(ctx context.Context, runID string) error {
	run, err := w.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("webhook: get run: %w", err)
	}

	if w.config.Events == EventFilterTerminalOnly && !isTerminal(run.State) {
		return nil
	}

	jobs, err := w.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("webhook: list jobs: %w", err)
	}

	payload := WebhookPayload{
		Event:      "run.state_changed",
		RunID:      run.ID,
		RepoID:     run.RepoID,
		Ref:        run.Ref,
		CommitSHA:  run.CommitSHA,
		State:      string(run.State),
		DurationMS: run.UpdatedAt.Sub(run.CreatedAt).Milliseconds(),
		CreatedAt:  run.CreatedAt,
		UpdatedAt:  run.UpdatedAt,
	}
	if w.dashboardURL != "" {
		payload.DashboardURL = fmt.Sprintf("%s/runs/%s", w.dashboardURL, run.ID)
	}
	for _, j := range jobs {
		payload.Jobs = append(payload.Jobs, JobSummary{
			Name:     j.Name,
			State:    string(j.State),
			Required: j.Required,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DeltaCI-Webhook/1.0")

	if w.config.Secret != "" {
		mac := hmac.New(sha256.New, []byte(w.config.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-DeltaCI-Signature", "sha256="+sig)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		w.logger.Warn("webhook endpoint returned non-2xx",
			"event", "webhook_error",
			"run_id", runID,
			"status", resp.StatusCode,
		)
	}
	return nil
}

func isTerminal(s state.RunState) bool {
	switch s {
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateCanceled,
		state.RunStateTimeout, state.RunStateReported:
		return true
	}
	return false
}
