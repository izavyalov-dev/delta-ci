package orchestrator

import (
	"testing"

	"github.com/izavyalov-dev/delta-ci/state"
)

func TestSanitizeRunDetailsInitializesArrays(t *testing.T) {
	details := RunDetails{
		Jobs: []JobDetail{
			{
				Artifacts:             nil,
				FailureExplanations:   nil,
				FailureAIExplanations: nil,
				FixSuggestions:        nil,
			},
		},
		Plan: &RunPlanDetail{
			SkippedJobs: nil,
		},
	}

	sanitized := sanitizeRunDetails(details)

	if sanitized.Jobs[0].Artifacts == nil {
		t.Fatalf("artifacts should be initialized")
	}
	if sanitized.Jobs[0].FailureExplanations == nil {
		t.Fatalf("failure explanations should be initialized")
	}
	if sanitized.Jobs[0].FailureAIExplanations == nil {
		t.Fatalf("failure ai explanations should be initialized")
	}
	if sanitized.Jobs[0].FixSuggestions == nil {
		t.Fatalf("fix suggestions should be initialized")
	}
	if sanitized.Plan == nil || sanitized.Plan.SkippedJobs == nil {
		t.Fatalf("skipped jobs should be initialized")
	}
}

func TestSanitizeRunDetailsClearsLeaseIDs(t *testing.T) {
	leaseID := "lease_123"
	details := RunDetails{
		Jobs: []JobDetail{
			{
				Attempts: []state.JobAttempt{
					{LeaseID: &leaseID},
				},
			},
		},
	}

	sanitized := sanitizeRunDetails(details)
	if sanitized.Jobs[0].Attempts[0].LeaseID != nil {
		t.Fatalf("lease id must be redacted")
	}
}

func TestParseJobPath(t *testing.T) {
	jobID, action, ok := parseJobPath("/api/v1/jobs/job_1/fix-suggestions")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if jobID != "job_1" || action != "fix-suggestions" {
		t.Fatalf("unexpected parse result: %q %q", jobID, action)
	}
}

func TestParseFixSuggestionPath(t *testing.T) {
	suggestionID, action, ok := parseFixSuggestionPath("/api/v1/fix-suggestions/42/validate")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if suggestionID != 42 || action != "validate" {
		t.Fatalf("unexpected parse result: %d %q", suggestionID, action)
	}
}
