package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func TestFixSuggestionEndpointsCreateAndValidate(t *testing.T) {
	ctx := context.Background()
	store, cleanup := setupTestStore(t, ctx)
	defer cleanup()

	service := newFixSuggestionTestService(store)
	run, err := service.CreateRun(ctx, CreateRunRequest{
		RepoID:    "repo",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if len(run.Jobs) == 0 {
		t.Fatalf("expected planned jobs")
	}
	jobID := run.Jobs[0].Job.ID

	handler := NewHTTPHandler(service, nil, HTTPConfig{})
	patch := `--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-fmt.Println("old")
+fmt.Println("new")
`

	body, err := json.Marshal(map[string]any{
		"provider":           "openai",
		"model":              "gpt-4o-mini",
		"prompt_version":     "fix-v1",
		"title":              "Update assertion",
		"summary":            "Adjust expected output",
		"patch_unified_diff": patch,
		"validate_now":       false,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/jobs/"+jobID+"/fix-suggestions", bytes.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createRec.Code, createRec.Body.String())
	}

	var created state.FixSuggestion
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ValidationStatus != state.FixSuggestionValidationPending {
		t.Fatalf("expected pending status, got %s", created.ValidationStatus)
	}
	if !created.RequiresApproval {
		t.Fatalf("expected requires_approval=true")
	}

	validateReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/fix-suggestions/%d/validate", created.ID), nil)
	validateRec := httptest.NewRecorder()
	handler.ServeHTTP(validateRec, validateReq)
	if validateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", validateRec.Code, validateRec.Body.String())
	}

	var validated state.FixSuggestion
	if err := json.NewDecoder(validateRec.Body).Decode(&validated); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if validated.ValidationStatus != state.FixSuggestionValidationQueued {
		t.Fatalf("expected queued status, got %s", validated.ValidationStatus)
	}
	if validated.ValidationRunID == nil || validated.ValidationJobID == nil {
		t.Fatalf("expected validation run/job ids")
	}
}

func TestFixSuggestionValidationStatusTransitions(t *testing.T) {
	ctx := context.Background()
	store, cleanup := setupTestStore(t, ctx)
	defer cleanup()

	service := newFixSuggestionTestService(store)
	run, err := service.CreateRun(ctx, CreateRunRequest{
		RepoID:    "repo",
		Ref:       "refs/heads/main",
		CommitSHA: "deadbeef",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	sourceJobID := run.Jobs[0].Job.ID

	tests := []struct {
		name           string
		patch          string
		completeStatus protocol.CompleteStatus
		summary        string
		expected       state.FixSuggestionValidationStatus
	}{
		{
			name: "succeeded",
			patch: `--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-fmt.Println("old")
+fmt.Println("new")
`,
			completeStatus: protocol.CompleteStatusSucceeded,
			summary:        "validation passed",
			expected:       state.FixSuggestionValidationSucceeded,
		},
		{
			name: "failed",
			patch: `--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-fmt.Println("old")
+fmt.Println("newer")
`,
			completeStatus: protocol.CompleteStatusFailed,
			summary:        "validation failed",
			expected:       state.FixSuggestionValidationFailed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			suggestion, err := service.CreateFixSuggestion(ctx, CreateFixSuggestionRequest{
				JobID:            sourceJobID,
				Provider:         "openai",
				Model:            "gpt-4o-mini",
				PromptVersion:    "fix-v1",
				Title:            "Validate fix",
				Summary:          "Run validation",
				PatchUnifiedDiff: tc.patch,
				ValidateNow:      true,
			})
			if err != nil {
				t.Fatalf("create fix suggestion: %v", err)
			}
			if suggestion.ValidationStatus != state.FixSuggestionValidationQueued {
				t.Fatalf("expected queued, got %s", suggestion.ValidationStatus)
			}
			if suggestion.ValidationJobID == nil {
				t.Fatalf("expected validation job id")
			}

			attempt := latestAttemptForJob(t, ctx, store, *suggestion.ValidationJobID)
			grant, err := service.GrantLease(ctx, GrantLeaseRequest{
				AttemptID:        attempt.ID,
				RunnerID:         "runner-1",
				TTLSeconds:       120,
				HeartbeatSeconds: 30,
			})
			if err != nil {
				t.Fatalf("grant lease: %v", err)
			}

			err = service.AckLease(ctx, protocol.AckLease{
				Type:       "AckLease",
				JobID:      *suggestion.ValidationJobID,
				LeaseID:    grant.LeaseID,
				RunnerID:   "runner-1",
				AcceptedAt: time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("ack lease: %v", err)
			}

			running, err := store.GetFixSuggestion(ctx, suggestion.ID)
			if err != nil {
				t.Fatalf("get running suggestion: %v", err)
			}
			if running.ValidationStatus != state.FixSuggestionValidationRunning {
				t.Fatalf("expected running status, got %s", running.ValidationStatus)
			}

			err = service.CompleteLease(ctx, protocol.Complete{
				Type:       "Complete",
				LeaseID:    grant.LeaseID,
				RunnerID:   "runner-1",
				Status:     tc.completeStatus,
				FinishedAt: time.Now().UTC(),
				Summary:    tc.summary,
			})
			if err != nil {
				t.Fatalf("complete lease: %v", err)
			}

			done, err := store.GetFixSuggestion(ctx, suggestion.ID)
			if err != nil {
				t.Fatalf("get completed suggestion: %v", err)
			}
			if done.ValidationStatus != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, done.ValidationStatus)
			}
			if !strings.Contains(done.ValidationSummary, tc.summary) {
				t.Fatalf("expected validation summary to contain %q, got %q", tc.summary, done.ValidationSummary)
			}
		})
	}
}

func newFixSuggestionTestService(store *state.Store) *Service {
	plan := stubPlanner{
		jobs: []planner.PlannedJob{
			{
				Name:     "build",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "build",
					Workdir: ".",
					Steps:   []string{"echo build"},
				},
				Reason: "test plan",
			},
		},
	}
	return NewService(store, plan, NoopDispatcher{}, &sequenceIDGen{}, nil, nil)
}
