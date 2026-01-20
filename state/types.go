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
	Reason       string    `json:"reason,omitempty"`
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

// SkippedJob captures jobs intentionally not scheduled in a plan.
type SkippedJob struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type FailureCategory string

const (
	FailureCategoryUser     FailureCategory = "USER"
	FailureCategoryInfra    FailureCategory = "INFRA"
	FailureCategoryTooling  FailureCategory = "TOOLING"
	FailureCategoryFlaky    FailureCategory = "FLAKY"
	FailureCategoryCanceled FailureCategory = "CANCELED"
	FailureCategoryUnknown  FailureCategory = "UNKNOWN"
)

type FailureConfidence string

const (
	FailureConfidenceLow    FailureConfidence = "LOW"
	FailureConfidenceMedium FailureConfidence = "MEDIUM"
	FailureConfidenceHigh   FailureConfidence = "HIGH"
)

// CacheEventSignal captures cache behavior relevant to failure analysis.
type CacheEventSignal struct {
	Type     string `json:"type"`
	Key      string `json:"key"`
	Hit      bool   `json:"hit"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

// FailureSignals capture deterministic inputs used for classification.
type FailureSignals struct {
	ExitCode        int                `json:"exit_code,omitempty"`
	AttemptNumber   int                `json:"attempt_number,omitempty"`
	DurationSeconds int                `json:"duration_seconds,omitempty"`
	CacheEvents     []CacheEventSignal `json:"cache_events,omitempty"`
	ArtifactTypes   []string           `json:"artifact_types,omitempty"`
	HasLog          bool               `json:"has_log,omitempty"`
}

func (s FailureSignals) IsEmpty() bool {
	return s.ExitCode == 0 &&
		s.AttemptNumber == 0 &&
		s.DurationSeconds == 0 &&
		len(s.CacheEvents) == 0 &&
		len(s.ArtifactTypes) == 0 &&
		!s.HasLog
}

// FailureExplanation summarizes why a job attempt failed.
type FailureExplanation struct {
	ID           int64             `json:"id"`
	JobAttemptID string            `json:"job_attempt_id"`
	Category     FailureCategory   `json:"category"`
	Summary      string            `json:"summary"`
	Confidence   FailureConfidence `json:"confidence"`
	Details      string            `json:"details,omitempty"`
	RuleVersion  string            `json:"rule_version,omitempty"`
	Signals      FailureSignals    `json:"signals,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// FailureAIExplanation stores advisory AI explanations for a failed attempt.
type FailureAIExplanation struct {
	ID            int64     `json:"id"`
	JobAttemptID  string    `json:"job_attempt_id"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model,omitempty"`
	PromptVersion string    `json:"prompt_version"`
	Summary       string    `json:"summary"`
	Details       string    `json:"details,omitempty"`
	LatencyMS     int       `json:"latency_ms,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// RunTrigger captures webhook metadata for idempotency and reporting.
type RunTrigger struct {
	RunID     string    `json:"run_id"`
	Provider  string    `json:"provider"`
	EventKey  string    `json:"event_key"`
	EventType string    `json:"event_type"`
	RepoID    string    `json:"repo_id"`
	RepoOwner string    `json:"repo_owner"`
	RepoName  string    `json:"repo_name"`
	PRNumber  *int      `json:"pr_number,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// StatusReport stores outbound VCS reporting metadata for a run.
type StatusReport struct {
	RunID       string    `json:"run_id"`
	Provider    string    `json:"provider"`
	CheckRunID  *string   `json:"check_run_id,omitempty"`
	PRCommentID *string   `json:"pr_comment_id,omitempty"`
	LastState   string    `json:"last_state"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
