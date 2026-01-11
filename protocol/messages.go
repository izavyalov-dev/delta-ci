package protocol

import "time"

type JobSpec struct {
	Name    string            `json:"name"`
	Workdir string            `json:"workdir,omitempty"`
	Steps   []string          `json:"steps"`
	Env     map[string]string `json:"env,omitempty"`
}

// LeaseGranted is sent from orchestrator to runner.
type LeaseGranted struct {
	Type                     string  `json:"type"` // always "LeaseGranted"
	RunID                    string  `json:"run_id"`
	JobID                    string  `json:"job_id"`
	LeaseID                  string  `json:"lease_id"`
	LeaseTTLSeconds          int     `json:"lease_ttl_seconds"`
	HeartbeatIntervalSeconds int     `json:"heartbeat_interval_seconds"`
	MaxRuntimeSeconds        int     `json:"max_runtime_seconds,omitempty"`
	JobSpec                  JobSpec `json:"job_spec"`
}

// AckLease is sent by a runner to acknowledge a lease.
type AckLease struct {
	Type       string    `json:"type"` // always "AckLease"
	JobID      string    `json:"job_id"`
	LeaseID    string    `json:"lease_id"`
	RunnerID   string    `json:"runner_id"`
	AcceptedAt time.Time `json:"accepted_at"`
}

// Heartbeat keeps a lease alive and reports optional progress.
type Heartbeat struct {
	Type     string    `json:"type"` // always "Heartbeat"
	LeaseID  string    `json:"lease_id"`
	RunnerID string    `json:"runner_id"`
	TS       time.Time `json:"ts"`
}

// HeartbeatAck is returned by the orchestrator in response to a heartbeat.
type HeartbeatAck struct {
	Type                  string `json:"type"` // always "HeartbeatAck"
	LeaseID               string `json:"lease_id"`
	ExtendLease           bool   `json:"extend_lease"`
	NewLeaseTTLSeconds    int    `json:"new_lease_ttl_seconds"`
	CancelRequested       bool   `json:"cancel_requested"`
	CancelDeadlineSeconds int    `json:"cancel_deadline_seconds"`
}

type CompleteStatus string

const (
	CompleteStatusSucceeded CompleteStatus = "SUCCEEDED"
	CompleteStatusFailed    CompleteStatus = "FAILED"
)

type ArtifactRef struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// Complete is sent by the runner when execution finishes.
type Complete struct {
	Type       string         `json:"type"` // always "Complete"
	LeaseID    string         `json:"lease_id"`
	RunnerID   string         `json:"runner_id"`
	Status     CompleteStatus `json:"status"`
	ExitCode   int            `json:"exit_code,omitempty"`
	FinishedAt time.Time      `json:"finished_at"`
	Summary    string         `json:"summary,omitempty"`
	Artifacts  []ArtifactRef  `json:"artifacts,omitempty"`
}

type CancelFinalStatus string

const (
	CancelFinalStatusCanceled CancelFinalStatus = "CANCELED"
)

// CancelAck is sent by a runner when a cancellation is acknowledged.
type CancelAck struct {
	Type        string            `json:"type"` // always "CancelAck"
	LeaseID     string            `json:"lease_id"`
	RunnerID    string            `json:"runner_id"`
	FinalStatus CancelFinalStatus `json:"final_status"`
	TS          time.Time         `json:"ts"`
	Artifacts   []ArtifactRef     `json:"artifacts,omitempty"`
	Summary     string            `json:"summary,omitempty"`
}
