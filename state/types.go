package state

import "time"

// Run represents a CI run.
type Run struct {
	ID        string    `json:"id"`
	RepoID    string    `json:"repo_id"`
	Ref       string    `json:"ref"`
	CommitSHA string    `json:"commit_sha"`
	State     RunState  `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Job represents a logical unit of work within a run.
type Job struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	Name         string    `json:"name"`
	Required     bool      `json:"required"`
	State        JobState  `json:"state"`
	AttemptCount int       `json:"attempt_count"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// JobAttempt represents a concrete execution attempt for a job.
type JobAttempt struct {
	ID            string     `json:"id"`
	JobID         string     `json:"job_id"`
	AttemptNumber int        `json:"attempt_number"`
	State         JobState   `json:"state"`
	LeaseID       *string    `json:"lease_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// Lease represents an execution lease for a job attempt.
type Lease struct {
	ID                       string     `json:"id"`
	JobAttemptID             string     `json:"job_attempt_id"`
	RunnerID                 *string    `json:"runner_id,omitempty"`
	State                    LeaseState `json:"state"`
	TTLSeconds               int        `json:"ttl_seconds"`
	HeartbeatIntervalSeconds int        `json:"heartbeat_interval_seconds"`
	GrantedAt                time.Time  `json:"granted_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	AcknowledgedAt           *time.Time `json:"acknowledged_at,omitempty"`
	LastHeartbeatAt          *time.Time `json:"last_heartbeat_at,omitempty"`
	ExpiresAt                *time.Time `json:"expires_at,omitempty"`
	CompletedAt              *time.Time `json:"completed_at,omitempty"`
}

// ArtifactRef is a lightweight reference to an external artifact.
type ArtifactRef struct {
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// Artifact represents a stored artifact reference for a job attempt.
type Artifact struct {
	ID           int64     `json:"id"`
	JobAttemptID string    `json:"job_attempt_id"`
	Type         string    `json:"type"`
	URI          string    `json:"uri"`
	CreatedAt    time.Time `json:"created_at"`
}
