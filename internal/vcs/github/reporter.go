package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/state"
)

// Reporter publishes run state to GitHub checks and PR comments.
type Reporter struct {
	store     *state.Store
	client    *Client
	logger    *slog.Logger
	checkName string
}

// NewReporter builds a GitHub reporter.
func NewReporter(store *state.Store, client *Client, logger *slog.Logger, checkName string) *Reporter {
	if logger == nil {
		logger = observability.NewLogger("status.github")
	}
	if checkName == "" {
		checkName = "delta-ci"
	}
	return &Reporter{
		store:     store,
		client:    client,
		logger:    logger,
		checkName: checkName,
	}
}

// ReportRun publishes the current run state to GitHub.
func (r *Reporter) ReportRun(ctx context.Context, runID string) error {
	if r == nil || r.store == nil {
		return nil
	}
	trigger, err := r.store.GetRunTrigger(ctx, runID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			return nil
		}
		return err
	}

	run, err := r.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.State == state.RunStateReported {
		return nil
	}

	report, err := r.store.GetStatusReport(ctx, runID, trigger.Provider)
	if err != nil && !errors.Is(err, state.ErrNotFound) {
		return err
	}
	if report.LastState == string(run.State) && report.LastState != "" {
		return nil
	}

	jobs, err := r.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return err
	}
	jobArtifacts := make(map[string][]state.Artifact, len(jobs))
	jobFailures := make(map[string]*state.FailureExplanation, len(jobs))
	for _, job := range jobs {
		artifacts, err := r.store.ListArtifactsByJob(ctx, job.ID)
		if err != nil {
			return err
		}
		jobArtifacts[job.ID] = artifacts

		explanations, err := r.store.ListFailureExplanationsByJob(ctx, job.ID)
		if err != nil {
			return err
		}
		if len(explanations) > 0 {
			jobFailures[job.ID] = &explanations[0]
		}
	}

	title, summary := buildSummary(run, jobs, jobArtifacts, jobFailures)
	checkReq := buildCheckRun(r.checkName, run, title, summary)

	checkRunID := report.CheckRunID
	if r.client == nil {
		return errors.New("github client not configured")
	}

	if checkRunID == nil {
		resp, err := r.client.CreateCheckRun(ctx, trigger.RepoOwner, trigger.RepoName, checkReq)
		if err != nil {
			r.logger.Warn("github check run create failed", "event", "github_check_create_failed", "run_id", run.ID, "state", run.State, "status", checkReq.Status, "conclusion", checkReq.Conclusion, "error", err)
			return err
		}
		value := fmt.Sprintf("%d", resp.ID)
		checkRunID = &value
	} else {
		_, err := r.client.UpdateCheckRun(ctx, trigger.RepoOwner, trigger.RepoName, *checkRunID, checkReq)
		if err != nil {
			if isNotFound(err) {
				resp, createErr := r.client.CreateCheckRun(ctx, trigger.RepoOwner, trigger.RepoName, checkReq)
				if createErr != nil {
					r.logger.Warn("github check run create failed", "event", "github_check_create_failed", "run_id", run.ID, "state", run.State, "status", checkReq.Status, "conclusion", checkReq.Conclusion, "error", createErr)
					return createErr
				}
				value := fmt.Sprintf("%d", resp.ID)
				checkRunID = &value
			} else {
				r.logger.Warn("github check run update failed", "event", "github_check_update_failed", "run_id", run.ID, "state", run.State, "status", checkReq.Status, "conclusion", checkReq.Conclusion, "error", err)
				return err
			}
		}
	}

	prCommentID := report.PRCommentID
	if trigger.PRNumber != nil && isTerminalState(run.State) {
		commentBody := buildComment(run, summary)
		if prCommentID == nil {
			resp, err := r.client.CreateComment(ctx, trigger.RepoOwner, trigger.RepoName, *trigger.PRNumber, commentBody)
			if err != nil {
				return err
			}
			value := fmt.Sprintf("%d", resp.ID)
			prCommentID = &value
		} else {
			_, err := r.client.UpdateComment(ctx, trigger.RepoOwner, trigger.RepoName, *prCommentID, commentBody)
			if err != nil {
				if isNotFound(err) {
					resp, createErr := r.client.CreateComment(ctx, trigger.RepoOwner, trigger.RepoName, *trigger.PRNumber, commentBody)
					if createErr != nil {
						return createErr
					}
					value := fmt.Sprintf("%d", resp.ID)
					prCommentID = &value
				} else {
					return err
				}
			}
		}
	}

	_, err = r.store.UpsertStatusReport(ctx, state.StatusReport{
		RunID:       runID,
		Provider:    trigger.Provider,
		CheckRunID:  checkRunID,
		PRCommentID: prCommentID,
		LastState:   string(run.State),
	})
	if err != nil {
		return err
	}

	if isReportableTerminal(run.State) {
		if err := r.store.TransitionRunState(ctx, run.ID, state.RunStateReported); err != nil {
			if !state.IsTransitionError(err) {
				return err
			}
		}
	}

	r.logger.Info("github status updated", "event", "github_status_updated", "run_id", run.ID, "state", run.State)
	return nil
}

func buildCheckRun(name string, run state.Run, title, summary string) CheckRunRequest {
	status, conclusion := mapRunToCheck(run.State)
	if status == "completed" && conclusion == "" {
		conclusion = "neutral"
	}
	req := CheckRunRequest{
		Name:    name,
		HeadSHA: run.CommitSHA,
		Status:  status,
	}
	req.Output.Title = title
	req.Output.Summary = summary
	if status == "completed" {
		req.Conclusion = conclusion
		completedAt := run.UpdatedAt
		req.CompletedAt = &completedAt
	}
	if status != "queued" {
		startedAt := run.CreatedAt
		req.StartedAt = &startedAt
	}
	return req
}

func mapRunToCheck(stateValue state.RunState) (string, string) {
	switch stateValue {
	case state.RunStateCreated, state.RunStatePlanning, state.RunStateQueued:
		return "queued", ""
	case state.RunStateRunning:
		return "in_progress", ""
	case state.RunStateSuccess:
		return "completed", "success"
	case state.RunStateFailed, state.RunStatePlanFailed:
		return "completed", "failure"
	case state.RunStateCanceled:
		return "completed", "cancelled"
	case state.RunStateTimeout:
		return "completed", "timed_out"
	default:
		return "in_progress", ""
	}
}

func isTerminalState(stateValue state.RunState) bool {
	switch stateValue {
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateCanceled, state.RunStateTimeout, state.RunStatePlanFailed:
		return true
	default:
		return false
	}
}

func isReportableTerminal(stateValue state.RunState) bool {
	switch stateValue {
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateCanceled, state.RunStateTimeout:
		return true
	default:
		return false
	}
}

func buildSummary(run state.Run, jobs []state.Job, artifacts map[string][]state.Artifact, failures map[string]*state.FailureExplanation) (string, string) {
	title := fmt.Sprintf("Delta CI: %s", run.State)
	var b strings.Builder
	fmt.Fprintf(&b, "Run `%s`\n\n", run.ID)
	fmt.Fprintf(&b, "State: `%s`\n", run.State)
	fmt.Fprintf(&b, "Ref: `%s`\n", run.Ref)
	fmt.Fprintf(&b, "Commit: `%s`\n", run.CommitSHA)
	if len(jobs) == 0 {
		return title, b.String()
	}

	b.WriteString("\nJobs:\n")
	for _, job := range jobs {
		required := "required"
		if !job.Required {
			required = "optional"
		}
		fmt.Fprintf(&b, "- %s (%s): `%s`\n", sanitize(job.Name), required, job.State)
		if job.State == state.JobStateFailed || job.State == state.JobStateTimedOut {
			if failure := failures[job.ID]; failure != nil {
				fmt.Fprintf(&b, "  Failure: %s (%s/%s)\n", sanitize(failure.Summary), sanitize(string(failure.Category)), sanitize(string(failure.Confidence)))
			}
		}
		arts := artifacts[job.ID]
		if len(arts) == 0 {
			continue
		}
		b.WriteString("  Artifacts: ")
		for i, art := range arts {
			if i > 0 {
				b.WriteString("; ")
			}
			fmt.Fprintf(&b, "%s %s", sanitize(art.Type), sanitize(art.URI))
		}
		b.WriteString("\n")
	}
	return title, b.String()
}

func buildComment(run state.Run, summary string) string {
	var b strings.Builder
	b.WriteString("<!-- delta-ci run:")
	b.WriteString(run.ID)
	b.WriteString(" -->\n")
	b.WriteString("## Delta CI Run Result\n\n")
	b.WriteString(summary)
	if !strings.HasSuffix(summary, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\nUpdated: ")
	b.WriteString(time.Now().UTC().Format(time.RFC3339))
	return b.String()
}

func sanitize(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	return value
}

func isNotFound(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}
