package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

// SlackConfig holds configuration for the Slack reporter.
type SlackConfig struct {
	WebhookURL string
	Events     EventFilter // "all" or "terminal-only" (default)
}

// SlackReporter sends Slack Block Kit messages via incoming webhook.
type SlackReporter struct {
	store        *state.Store
	client       *http.Client
	config       SlackConfig
	logger       *slog.Logger
	dashboardURL string
}

// NewSlackReporter creates a SlackReporter.
func NewSlackReporter(store *state.Store, config SlackConfig, logger *slog.Logger, dashboardURL string) *SlackReporter {
	if config.Events == "" {
		config.Events = EventFilterTerminalOnly
	}
	return &SlackReporter{
		store:        store,
		client:       &http.Client{Timeout: 10 * time.Second},
		config:       config,
		logger:       logger,
		dashboardURL: dashboardURL,
	}
}

func (s *SlackReporter) ReportRun(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("slack: get run: %w", err)
	}

	if s.config.Events == EventFilterTerminalOnly && !isTerminal(run.State) {
		return nil
	}

	jobs, err := s.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return fmt.Errorf("slack: list jobs: %w", err)
	}

	msg := s.buildMessage(run, jobs)
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("slack: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		s.logger.Warn("slack webhook returned non-2xx",
			"event", "slack_error",
			"run_id", runID,
			"status", resp.StatusCode,
		)
	}
	return nil
}

type slackMessage struct {
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type     string      `json:"type"`
	Text     *slackText  `json:"text,omitempty"`
	Elements []slackText `json:"elements,omitempty"`
}

type slackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *SlackReporter) buildMessage(run state.Run, jobs []state.Job) slackMessage {
	emoji := stateEmoji(run.State)
	duration := run.UpdatedAt.Sub(run.CreatedAt).Round(time.Second)

	header := fmt.Sprintf("%s *%s* — `%s` on `%s`", emoji, run.State, run.RepoID, run.Ref)

	var jobLines []string
	for _, j := range jobs {
		je := stateEmoji(state.RunState(j.State))
		jobLines = append(jobLines, fmt.Sprintf("%s %s: `%s`", je, j.Name, j.State))
	}

	blocks := []slackBlock{
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: header}},
		{Type: "section", Text: &slackText{Type: "mrkdwn", Text: fmt.Sprintf("*Commit:* `%s`\n*Duration:* %s", run.CommitSHA, duration)}},
	}

	if len(jobLines) > 0 {
		blocks = append(blocks, slackBlock{
			Type: "section",
			Text: &slackText{Type: "mrkdwn", Text: "*Jobs:*\n" + strings.Join(jobLines, "\n")},
		})
	}

	if s.dashboardURL != "" {
		blocks = append(blocks, slackBlock{
			Type: "context",
			Elements: []slackText{
				{Type: "mrkdwn", Text: fmt.Sprintf("<%s/runs/%s|View in Dashboard>", s.dashboardURL, run.ID)},
			},
		})
	}

	return slackMessage{Blocks: blocks}
}

func stateEmoji(s state.RunState) string {
	switch s {
	case state.RunStateSuccess:
		return "\u2705" // checkmark
	case state.RunStateFailed:
		return "\u274c" // x
	case state.RunStateCanceled:
		return "\u23f9\ufe0f" // stop
	case state.RunStateTimeout:
		return "\u23f0" // alarm clock
	case state.RunStateRunning:
		return "\u25b6\ufe0f" // play
	default:
		return "\u2139\ufe0f" // info
	}
}
