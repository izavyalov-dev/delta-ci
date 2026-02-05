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

	_, summary := buildSummary(run, nil, jobs, artifacts, failures, ai)
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

	_, summary := buildSummary(run, nil, jobs, map[string][]state.Artifact{}, failures, map[string]*state.FailureAIExplanation{})
	if strings.Contains(summary, "AI advisory:") {
		t.Fatalf("did not expect AI advisory in summary: %s", summary)
	}
}
