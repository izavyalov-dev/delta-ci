package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

const (
	defaultMaxFailureSummaryLen = 160
	defaultMaxFailureDetailsLen = 512
)

// FailureInput captures sanitized inputs for rule-based analysis.
type FailureInput struct {
	RunID     string
	JobID     string
	JobName   string
	AttemptID string
	Status    protocol.CompleteStatus
	ExitCode  int
	Summary   string
	Artifacts []state.ArtifactRef
}

// FailureAnalyzer produces a failure explanation for a job attempt.
type FailureAnalyzer interface {
	Analyze(ctx context.Context, input FailureInput) (*state.FailureExplanation, error)
}

// FailureAdvisor is an optional AI hook for advisory explanations.
type FailureAdvisor interface {
	Explain(ctx context.Context, input FailureInput) (string, error)
}

// NoopFailureAnalyzer disables failure analysis.
type NoopFailureAnalyzer struct{}

func (NoopFailureAnalyzer) Analyze(ctx context.Context, input FailureInput) (*state.FailureExplanation, error) {
	return nil, nil
}

// RuleBasedFailureAnalyzer classifies failures via simple heuristics.
type RuleBasedFailureAnalyzer struct {
	MaxSummaryLen int
	MaxDetailsLen int
	Advisor       FailureAdvisor
	EnableAI      bool
}

// NewRuleBasedFailureAnalyzer returns a default rule-based analyzer.
func NewRuleBasedFailureAnalyzer() *RuleBasedFailureAnalyzer {
	return &RuleBasedFailureAnalyzer{
		MaxSummaryLen: defaultMaxFailureSummaryLen,
		MaxDetailsLen: defaultMaxFailureDetailsLen,
	}
}

func (a *RuleBasedFailureAnalyzer) Analyze(ctx context.Context, input FailureInput) (*state.FailureExplanation, error) {
	if input.AttemptID == "" {
		return nil, fmt.Errorf("attempt id required for failure analysis")
	}
	if input.Status != protocol.CompleteStatusFailed {
		return nil, nil
	}

	summary := sanitizeText(input.Summary, a.MaxSummaryLen)
	category, confidence, concise := classifyFailure(input.JobName, summary, input.ExitCode)

	details := buildFailureDetails(input, summary, a.MaxDetailsLen)
	if a.EnableAI && a.Advisor != nil {
		if aiSummary, err := a.Advisor.Explain(ctx, FailureInput{
			RunID:     input.RunID,
			JobID:     input.JobID,
			JobName:   input.JobName,
			AttemptID: input.AttemptID,
			Status:    input.Status,
			ExitCode:  input.ExitCode,
			Summary:   summary,
			Artifacts: input.Artifacts,
		}); err == nil {
			aiSummary = sanitizeText(aiSummary, a.MaxDetailsLen)
			if aiSummary != "" {
				details = appendDetail(details, "AI advisory: "+aiSummary, a.MaxDetailsLen)
			}
		}
	}

	return &state.FailureExplanation{
		JobAttemptID: input.AttemptID,
		Category:     category,
		Confidence:   confidence,
		Summary:      concise,
		Details:      details,
	}, nil
}

func classifyFailure(jobName, summary string, exitCode int) (state.FailureCategory, state.FailureConfidence, string) {
	name := strings.ToLower(jobName)
	lower := strings.ToLower(summary)

	switch {
	case containsAny(lower, "timed out", "timeout", "deadline exceeded") || exitCode == 124:
		return state.FailureCategoryInfra, state.FailureConfidenceMedium, fmt.Sprintf("Job timed out (exit code %d).", exitCode)
	case containsAny(lower, "out of memory", "no space", "disk full", "signal: killed", "killed"):
		return state.FailureCategoryInfra, state.FailureConfidenceHigh, fmt.Sprintf("Resource exhaustion detected (exit code %d).", exitCode)
	case containsAny(lower, "dial tcp", "connection refused", "i/o timeout", "temporary failure", "tls handshake timeout"):
		return state.FailureCategoryInfra, state.FailureConfidenceHigh, fmt.Sprintf("Network error detected (exit code %d).", exitCode)
	case containsAny(lower, "command not found", "executable file not found", "no such file or directory"):
		return state.FailureCategoryTooling, state.FailureConfidenceHigh, fmt.Sprintf("Missing tool or script (exit code %d).", exitCode)
	case containsAny(lower, "permission denied"):
		return state.FailureCategoryTooling, state.FailureConfidenceMedium, fmt.Sprintf("Permission error detected (exit code %d).", exitCode)
	case strings.Contains(name, "lint") || strings.Contains(name, "vet"):
		return state.FailureCategoryUser, state.FailureConfidenceMedium, fmt.Sprintf("Lint step failed (exit code %d).", exitCode)
	case strings.Contains(name, "test"):
		return state.FailureCategoryUser, state.FailureConfidenceMedium, fmt.Sprintf("Test step failed (exit code %d).", exitCode)
	case strings.Contains(name, "build"):
		return state.FailureCategoryUser, state.FailureConfidenceMedium, fmt.Sprintf("Build step failed (exit code %d).", exitCode)
	default:
		return state.FailureCategoryUser, state.FailureConfidenceLow, fmt.Sprintf("Job failed (exit code %d).", exitCode)
	}
}

func buildFailureDetails(input FailureInput, summary string, maxLen int) string {
	details := ""
	if summary != "" && !isGenericSummary(summary) {
		details = appendDetail(details, "Observed: "+summary, maxLen)
	}
	if input.ExitCode != 0 {
		details = appendDetail(details, fmt.Sprintf("Exit code: %d", input.ExitCode), maxLen)
	}
	for _, artifact := range input.Artifacts {
		if strings.EqualFold(artifact.Type, "log") {
			details = appendDetail(details, "Log: "+sanitizeText(artifact.URI, maxLen), maxLen)
			break
		}
	}
	return details
}

func appendDetail(existing, next string, maxLen int) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return existing
	}
	if existing == "" {
		return truncateText(next, maxLen)
	}
	combined := existing + " | " + next
	return truncateText(combined, maxLen)
}

func sanitizeText(value string, maxLen int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.TrimSpace(value)
	value = strings.Join(strings.Fields(value), " ")
	return truncateText(value, maxLen)
}

func truncateText(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	if maxLen <= 3 {
		return value[:maxLen]
	}
	return value[:maxLen-3] + "..."
}

func isGenericSummary(summary string) bool {
	lower := strings.ToLower(summary)
	if strings.HasPrefix(lower, "exit status ") {
		return true
	}
	return false
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
