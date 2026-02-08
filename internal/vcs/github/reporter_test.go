package github

import (
	"strings"
	"testing"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

func TestBuildSummaryIncludesAIAdvisoryAndEvidence(t *testing.T) {
	now := time.Now().UTC()
	run := state.Run{
		ID:        "run_1",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
		State:     state.RunStateFailed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	jobs := []state.Job{
		{
			ID:       "job_1",
			Name:     "test",
			Required: true,
			State:    state.JobStateFailed,
		},
	}
	artifacts := map[string][]state.Artifact{
		"job_1": []state.Artifact{
			{Type: "log", URI: "s3://bucket/log.txt"},
		},
	}
	failures := map[string]*state.FailureExplanation{
		"job_1": {
			Summary:    "Test step failed (exit code 1).",
			Category:   state.FailureCategoryUser,
			Confidence: state.FailureConfidenceMedium,
			Signals: state.FailureSignals{
				ExitCode:      1,
				AttemptNumber: 1,
			},
		},
	}
	ai := map[string]*state.FailureAIExplanation{
		"job_1": {
			Provider:      "openai",
			Model:         "gpt-4o-mini",
			PromptVersion: "failure-explain-v1",
			Summary:       "Likely assertion mismatch in unit tests.",
		},
	}

	_, summary := buildSummary(run, nil, jobs, artifacts, failures, ai, map[string]*state.FixSuggestion{})
	if !strings.Contains(summary, "AI advisory:") {
		t.Fatalf("expected AI advisory in summary: %s", summary)
	}
	if !strings.Contains(summary, "openai/gpt-4o-mini/failure-explain-v1") {
		t.Fatalf("expected AI metadata in summary: %s", summary)
	}
	if !strings.Contains(summary, "Evidence: s3://bucket/log.txt") {
		t.Fatalf("expected evidence link in summary: %s", summary)
	}
}

func TestBuildSummaryWithoutAIAdvisory(t *testing.T) {
	now := time.Now().UTC()
	run := state.Run{
		ID:        "run_2",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
		State:     state.RunStateFailed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	jobs := []state.Job{
		{
			ID:       "job_2",
			Name:     "build",
			Required: true,
			State:    state.JobStateFailed,
		},
	}
	failures := map[string]*state.FailureExplanation{
		"job_2": {
			Summary:    "Build step failed (exit code 2).",
			Category:   state.FailureCategoryUser,
			Confidence: state.FailureConfidenceLow,
		},
	}

	_, summary := buildSummary(run, nil, jobs, map[string][]state.Artifact{}, failures, map[string]*state.FailureAIExplanation{}, map[string]*state.FixSuggestion{})
	if strings.Contains(summary, "AI advisory:") {
		t.Fatalf("did not expect AI advisory in summary: %s", summary)
	}
}

func TestBuildSummaryIncludesFixValidation(t *testing.T) {
	now := time.Now().UTC()
	run := state.Run{
		ID:        "run_3",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
		State:     state.RunStateFailed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	jobs := []state.Job{
		{
			ID:       "job_3",
			Name:     "lint",
			Required: true,
			State:    state.JobStateFailed,
		},
	}
	fixRunID := "run_fix_1"
	fixJobID := "job_fix_1"
	fixes := map[string]*state.FixSuggestion{
		"job_3": {
			ID:                42,
			ValidationStatus:  state.FixSuggestionValidationRunning,
			ValidationSummary: "replaying patch on latest base commit",
			ValidationRunID:   &fixRunID,
			ValidationJobID:   &fixJobID,
			RequiresApproval:  true,
		},
	}

	_, summary := buildSummary(
		run,
		nil,
		jobs,
		map[string][]state.Artifact{},
		map[string]*state.FailureExplanation{},
		map[string]*state.FailureAIExplanation{},
		fixes,
	)
	if !strings.Contains(summary, "Fix validation: suggestion #42 VALIDATION_RUNNING") {
		t.Fatalf("expected fix validation status in summary: %s", summary)
	}
	if !strings.Contains(summary, "replaying patch on latest base commit") {
		t.Fatalf("expected fix validation summary in output: %s", summary)
	}
	if !strings.Contains(summary, "run=run_fix_1 job=job_fix_1") {
		t.Fatalf("expected fix validation references in output: %s", summary)
	}
	if !strings.Contains(summary, "approval required") {
		t.Fatalf("expected approval marker in output: %s", summary)
	}
}

func TestSummarizeFixValidationStatuses(t *testing.T) {
	testCases := []struct {
		name   string
		status state.FixSuggestionValidationStatus
	}{
		{name: "queued", status: state.FixSuggestionValidationQueued},
		{name: "running", status: state.FixSuggestionValidationRunning},
		{name: "succeeded", status: state.FixSuggestionValidationSucceeded},
		{name: "failed", status: state.FixSuggestionValidationFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			summary := summarizeFixValidation(state.FixSuggestion{
				ID:               7,
				ValidationStatus: tc.status,
			})
			expected := "suggestion #7 " + string(tc.status)
			if !strings.Contains(summary, expected) {
				t.Fatalf("expected %q in summary %q", expected, summary)
			}
		})
	}
}
