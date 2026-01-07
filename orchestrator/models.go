package orchestrator

import "github.com/izavyalov-dev/delta-ci/state"

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
	Run  state.Run   `json:"run"`
	Jobs []JobDetail `json:"jobs"`
}

// JobDetail presents a job alongside its attempts.
type JobDetail struct {
	Job      state.Job          `json:"job"`
	Attempts []state.JobAttempt `json:"attempts"`
}
