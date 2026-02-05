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
