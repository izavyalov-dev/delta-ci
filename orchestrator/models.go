package orchestrator

import (
	"time"

	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

// CreateRunRequest captures inputs to start a new run.
type CreateRunRequest struct {
	RepoID    string
	Ref       string
	CommitSHA string
}

// GrantLeaseRequest describes parameters to grant a lease to a runner.
type GrantLeaseRequest struct {
	AttemptID         string
	RunnerID          string
	TTLSeconds        int
	HeartbeatSeconds  int
	MaxRuntimeSeconds int
}

// RunDetails aggregates run, jobs, and attempts for read-only APIs.
type RunDetails struct {
	Run  state.Run      `json:"run"`
	Jobs []JobDetail    `json:"jobs"`
	Plan *RunPlanDetail `json:"plan,omitempty"`
}

// JobDetail presents a job alongside its attempts.
type JobDetail struct {
	Job                   state.Job                    `json:"job"`
	Spec                  *protocol.JobSpec            `json:"spec,omitempty"`
	Attempts              []state.JobAttempt           `json:"attempts"`
	Artifacts             []state.Artifact             `json:"artifacts"`
	FailureExplanations   []state.FailureExplanation   `json:"failure_explanations"`
	FailureAIExplanations []state.FailureAIExplanation `json:"failure_ai_explanations"`
	FixSuggestions        []state.FixSuggestion        `json:"fix_suggestions"`
}

// RunPlanDetail provides plan explainability metadata for APIs.
type RunPlanDetail struct {
	RecipeSource    string             `json:"recipe_source"`
	RecipeID        *string            `json:"recipe_id,omitempty"`
	RecipeVersion   *int               `json:"recipe_version,omitempty"`
	Fingerprint     string             `json:"fingerprint,omitempty"`
	Explain         string             `json:"explain,omitempty"`
	SkippedJobs     []state.SkippedJob `json:"skipped_jobs"`
	DetectedPlugins []string           `json:"detected_plugins,omitempty"`
}

// RunFilter describes filtering and pagination for run list queries.
type RunFilter struct {
	Repo   string
	State  string
	Limit  int
	Offset int
}

// RunSummary is a lightweight run view for list pages.
type RunSummary struct {
	ID        string         `json:"id"`
	RepoID    string         `json:"repo_id"`
	Ref       string         `json:"ref"`
	CommitSHA string         `json:"commit_sha"`
	State     state.RunState `json:"state"`
	JobCount  int            `json:"job_count"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// SystemStats provides aggregate metrics for the dashboard.
type SystemStats struct {
	Total     int `json:"total"`
	Running   int `json:"running"`
	Queued    int `json:"queued"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Canceled  int `json:"canceled"`
}
