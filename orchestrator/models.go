package orchestrator

import "github.com/izavyalov-dev/delta-ci/state"

// CreateRunRequest captures inputs to start a new run.
type CreateRunRequest struct {
	RepoID    string
	Ref       string
	CommitSHA string
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
