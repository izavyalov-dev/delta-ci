package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// CreateRun inserts a new run in CREATED state unless explicitly provided.
func (s *Store) CreateRun(ctx context.Context, run Run) (Run, error) {
	if run.State == "" {
		run.State = RunStateCreated
	}

	err := s.db.QueryRowContext(ctx, `
INSERT INTO runs (id, repo_id, ref, commit_sha, state)
VALUES ($1, $2, $3, $4, $5)
RETURNING created_at, updated_at
`, run.ID, run.RepoID, run.Ref, run.CommitSHA, run.State).Scan(&run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return Run{}, err
	}

	return run, nil
}

// GetRun returns a single run by ID.
func (s *Store) GetRun(ctx context.Context, runID string) (Run, error) {
	var run Run
	err := s.db.QueryRowContext(ctx, `
SELECT id, repo_id, ref, commit_sha, state, created_at, updated_at
FROM runs
WHERE id = $1
`, runID).Scan(&run.ID, &run.RepoID, &run.Ref, &run.CommitSHA, &run.State, &run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, fmt.Errorf("%w: run %s", ErrNotFound, runID)
		}
		return Run{}, err
	}
	return run, nil
}

// GetJob returns a single job by ID.
func (s *Store) GetJob(ctx context.Context, jobID string) (Job, error) {
	var job Job
	var reason sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT id, run_id, name, required, state, attempt_count, reason, created_at, updated_at
FROM jobs
WHERE id = $1
`, jobID).Scan(&job.ID, &job.RunID, &job.Name, &job.Required, &job.State, &job.AttemptCount, &reason, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, fmt.Errorf("%w: job %s", ErrNotFound, jobID)
		}
		return Job{}, err
	}
	if reason.Valid {
		job.Reason = reason.String
	}
	return job, nil
}

// CreateJob inserts a new job in CREATED state unless explicitly provided.
func (s *Store) CreateJob(ctx context.Context, job Job) (Job, error) {
	if job.State == "" {
		job.State = JobStateCreated
	}

	err := s.db.QueryRowContext(ctx, `
INSERT INTO jobs (id, run_id, name, required, state, attempt_count, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING created_at, updated_at
`, job.ID, job.RunID, job.Name, job.Required, job.State, job.AttemptCount, nullableString(job.Reason)).Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}

	return job, nil
}

// ListJobsByRun returns all jobs for a given run ordered by creation time.
func (s *Store) ListJobsByRun(ctx context.Context, runID string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, name, required, state, attempt_count, reason, created_at, updated_at
FROM jobs
WHERE run_id = $1
ORDER BY created_at ASC, id ASC
`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var reason sql.NullString
		if err := rows.Scan(&job.ID, &job.RunID, &job.Name, &job.Required, &job.State, &job.AttemptCount, &reason, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		if reason.Valid {
			job.Reason = reason.String
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// CreateJobAttempt inserts a new job attempt and updates the job's attempt count.
func (s *Store) CreateJobAttempt(ctx context.Context, attempt JobAttempt) (JobAttempt, error) {
	if attempt.State == "" {
		attempt.State = JobStateCreated
	}
	if attempt.AttemptNumber == 0 {
		attempt.AttemptNumber = 1
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
INSERT INTO job_attempts (id, job_id, attempt_number, state, lease_id, started_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING created_at, updated_at
`, attempt.ID, attempt.JobID, attempt.AttemptNumber, attempt.State, attempt.LeaseID, attempt.StartedAt, attempt.CompletedAt).
			Scan(&attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE jobs
SET attempt_count = GREATEST(attempt_count, $2), updated_at = NOW()
WHERE id = $1
`, attempt.JobID, attempt.AttemptNumber); err != nil {
			return err
		}
		return nil
	})

	return attempt, err
}

// ListJobAttempts returns all attempts for a job ordered by attempt_number.
func (s *Store) ListJobAttempts(ctx context.Context, jobID string) ([]JobAttempt, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, job_id, attempt_number, state, lease_id, created_at, updated_at, started_at, completed_at
FROM job_attempts
WHERE job_id = $1
ORDER BY attempt_number ASC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attempts []JobAttempt
	for rows.Next() {
		var attempt JobAttempt
		var leaseID sql.NullString
		var startedAt sql.NullTime
		var completedAt sql.NullTime
		if err := rows.Scan(&attempt.ID, &attempt.JobID, &attempt.AttemptNumber, &attempt.State, &leaseID, &attempt.CreatedAt, &attempt.UpdatedAt, &startedAt, &completedAt); err != nil {
			return nil, err
		}
		if leaseID.Valid {
			attempt.LeaseID = &leaseID.String
		}
		if startedAt.Valid {
			attempt.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			attempt.CompletedAt = &completedAt.Time
		}
		attempts = append(attempts, attempt)
	}

	return attempts, rows.Err()
}

// GetJobAttempt returns a single attempt by ID.
func (s *Store) GetJobAttempt(ctx context.Context, attemptID string) (JobAttempt, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_id, attempt_number, state, lease_id, created_at, updated_at, started_at, completed_at
FROM job_attempts
WHERE id = $1
`, attemptID)

	var attempt JobAttempt
	var leaseID sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	if err := row.Scan(&attempt.ID, &attempt.JobID, &attempt.AttemptNumber, &attempt.State, &leaseID, &attempt.CreatedAt, &attempt.UpdatedAt, &startedAt, &completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobAttempt{}, fmt.Errorf("%w: job attempt %s", ErrNotFound, attemptID)
		}
		return JobAttempt{}, err
	}
	if leaseID.Valid {
		attempt.LeaseID = &leaseID.String
	}
	if startedAt.Valid {
		attempt.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		attempt.CompletedAt = &completedAt.Time
	}
	return attempt, nil
}

// GetLatestJobAttempt returns the most recent attempt for a job.
func (s *Store) GetLatestJobAttempt(ctx context.Context, jobID string) (JobAttempt, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_id, attempt_number, state, lease_id, created_at, updated_at, started_at, completed_at
FROM job_attempts
WHERE job_id = $1
ORDER BY attempt_number DESC
LIMIT 1
`, jobID)

	var attempt JobAttempt
	var leaseID sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	if err := row.Scan(&attempt.ID, &attempt.JobID, &attempt.AttemptNumber, &attempt.State, &leaseID, &attempt.CreatedAt, &attempt.UpdatedAt, &startedAt, &completedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return JobAttempt{}, fmt.Errorf("%w: job attempt %s", ErrNotFound, jobID)
		}
		return JobAttempt{}, err
	}
	if leaseID.Valid {
		attempt.LeaseID = &leaseID.String
	}
	if startedAt.Valid {
		attempt.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		attempt.CompletedAt = &completedAt.Time
	}
	return attempt, nil
}

// CreateLease inserts a new lease record.
func (s *Store) CreateLease(ctx context.Context, lease Lease) (Lease, error) {
	if lease.State == "" {
		lease.State = LeaseStateGranted
	}
	if lease.TTLSeconds == 0 {
		return Lease{}, fmt.Errorf("ttl_seconds must be > 0")
	}
	if lease.HeartbeatIntervalSeconds == 0 {
		return Lease{}, fmt.Errorf("heartbeat_interval_seconds must be > 0")
	}
	if lease.TTLSeconds <= lease.HeartbeatIntervalSeconds {
		return Lease{}, fmt.Errorf("ttl_seconds must be greater than heartbeat_interval_seconds")
	}

	err := s.db.QueryRowContext(ctx, `
INSERT INTO leases (id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, acknowledged_at, last_heartbeat_at, expires_at, completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING granted_at, updated_at
`, lease.ID, lease.JobAttemptID, lease.RunnerID, lease.State, lease.TTLSeconds, lease.HeartbeatIntervalSeconds, lease.AcknowledgedAt, lease.LastHeartbeatAt, lease.ExpiresAt, lease.CompletedAt).
		Scan(&lease.GrantedAt, &lease.UpdatedAt)
	if err != nil {
		return Lease{}, err
	}

	return lease, nil
}

// GetLease returns a single lease by ID.
func (s *Store) GetLease(ctx context.Context, leaseID string) (Lease, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, granted_at, updated_at, acknowledged_at, last_heartbeat_at, expires_at, completed_at
FROM leases
WHERE id = $1
`, leaseID)

	var lease Lease
	var runnerID sql.NullString
	var acked sql.NullTime
	var lastHB sql.NullTime
	var expires sql.NullTime
	var completed sql.NullTime
	if err := row.Scan(
		&lease.ID,
		&lease.JobAttemptID,
		&runnerID,
		&lease.State,
		&lease.TTLSeconds,
		&lease.HeartbeatIntervalSeconds,
		&lease.GrantedAt,
		&lease.UpdatedAt,
		&acked,
		&lastHB,
		&expires,
		&completed,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Lease{}, fmt.Errorf("%w: lease %s", ErrNotFound, leaseID)
		}
		return Lease{}, err
	}
	if runnerID.Valid {
		lease.RunnerID = &runnerID.String
	}
	if acked.Valid {
		lease.AcknowledgedAt = &acked.Time
	}
	if lastHB.Valid {
		lease.LastHeartbeatAt = &lastHB.Time
	}
	if expires.Valid {
		lease.ExpiresAt = &expires.Time
	}
	if completed.Valid {
		lease.CompletedAt = &completed.Time
	}
	return lease, nil
}

// MarkJobAttemptStarted sets the started_at timestamp.
func (s *Store) MarkJobAttemptStarted(ctx context.Context, attemptID string, started time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE job_attempts
SET started_at = $2, updated_at = NOW()
WHERE id = $1
`, attemptID, started)
	return err
}

// MarkJobAttemptCompleted sets the completed_at timestamp.
func (s *Store) MarkJobAttemptCompleted(ctx context.Context, attemptID string, completed time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE job_attempts
SET completed_at = $2, updated_at = NOW()
WHERE id = $1
`, attemptID, completed)
	return err
}

// RecordArtifacts persists artifact references for a job attempt.
func (s *Store) RecordArtifacts(ctx context.Context, attemptID string, refs []ArtifactRef) error {
	if attemptID == "" {
		return errors.New("attempt id required")
	}
	if len(refs) == 0 {
		return nil
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, ref := range refs {
			if ref.Type == "" || ref.URI == "" {
				return errors.New("artifact refs require type and uri")
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO job_artifacts (job_attempt_id, artifact_type, uri)
VALUES ($1, $2, $3)
ON CONFLICT (job_attempt_id, uri) DO NOTHING
`, attemptID, ref.Type, ref.URI); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListArtifactsByJob returns all artifact references for a job across attempts.
func (s *Store) ListArtifactsByJob(ctx context.Context, jobID string) ([]Artifact, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.job_attempt_id, a.artifact_type, a.uri, a.created_at
FROM job_artifacts a
JOIN job_attempts ja ON ja.id = a.job_attempt_id
WHERE ja.job_id = $1
ORDER BY a.created_at ASC, a.id ASC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var artifact Artifact
		if err := rows.Scan(&artifact.ID, &artifact.JobAttemptID, &artifact.Type, &artifact.URI, &artifact.CreatedAt); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, rows.Err()
}

// RecordFailureExplanation persists a failure explanation for a job attempt.
func (s *Store) RecordFailureExplanation(ctx context.Context, explanation FailureExplanation) error {
	if explanation.JobAttemptID == "" {
		return errors.New("job attempt id required")
	}
	if explanation.Category == "" {
		return errors.New("failure category required")
	}
	if explanation.Summary == "" {
		return errors.New("failure summary required")
	}
	if explanation.Confidence == "" {
		explanation.Confidence = FailureConfidenceLow
	}

	signalsJSON, err := marshalFailureSignals(explanation.Signals)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO job_failure_explanations (job_attempt_id, category, summary, confidence, details, rule_version, signals)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (job_attempt_id)
DO UPDATE SET category = EXCLUDED.category,
              summary = EXCLUDED.summary,
              confidence = EXCLUDED.confidence,
              details = EXCLUDED.details,
              rule_version = EXCLUDED.rule_version,
              signals = EXCLUDED.signals,
              created_at = NOW()
`, explanation.JobAttemptID, explanation.Category, explanation.Summary, explanation.Confidence, nullableString(explanation.Details), nullableString(explanation.RuleVersion), signalsJSON)
	return err
}

// GetFailureExplanationByAttempt fetches a failure explanation for a job attempt.
func (s *Store) GetFailureExplanationByAttempt(ctx context.Context, attemptID string) (FailureExplanation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_attempt_id, category, summary, confidence, details, rule_version, signals, created_at
FROM job_failure_explanations
WHERE job_attempt_id = $1
`, attemptID)

	var explanation FailureExplanation
	var details sql.NullString
	var ruleVersion sql.NullString
	var signalsJSON []byte
	if err := row.Scan(&explanation.ID, &explanation.JobAttemptID, &explanation.Category, &explanation.Summary, &explanation.Confidence, &details, &ruleVersion, &signalsJSON, &explanation.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FailureExplanation{}, fmt.Errorf("%w: failure explanation for attempt %s", ErrNotFound, attemptID)
		}
		return FailureExplanation{}, err
	}
	if details.Valid {
		explanation.Details = details.String
	}
	if ruleVersion.Valid {
		explanation.RuleVersion = ruleVersion.String
	}
	if len(signalsJSON) > 0 {
		signals, err := unmarshalFailureSignals(signalsJSON)
		if err != nil {
			return FailureExplanation{}, err
		}
		explanation.Signals = signals
	}
	return explanation, nil
}

// ListFailureExplanationsByJob returns failure explanations for a job.
func (s *Store) ListFailureExplanationsByJob(ctx context.Context, jobID string) ([]FailureExplanation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.id, e.job_attempt_id, e.category, e.summary, e.confidence, e.details, e.rule_version, e.signals, e.created_at
FROM job_failure_explanations e
JOIN job_attempts ja ON ja.id = e.job_attempt_id
WHERE ja.job_id = $1
ORDER BY e.created_at DESC, e.id DESC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var explanations []FailureExplanation
	for rows.Next() {
		var explanation FailureExplanation
		var details sql.NullString
		var ruleVersion sql.NullString
		var signalsJSON []byte
		if err := rows.Scan(&explanation.ID, &explanation.JobAttemptID, &explanation.Category, &explanation.Summary, &explanation.Confidence, &details, &ruleVersion, &signalsJSON, &explanation.CreatedAt); err != nil {
			return nil, err
		}
		if details.Valid {
			explanation.Details = details.String
		}
		if ruleVersion.Valid {
			explanation.RuleVersion = ruleVersion.String
		}
		if len(signalsJSON) > 0 {
			signals, err := unmarshalFailureSignals(signalsJSON)
			if err != nil {
				return nil, err
			}
			explanation.Signals = signals
		}
		explanations = append(explanations, explanation)
	}

	return explanations, rows.Err()
}

func nullableString(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func marshalFailureSignals(signals FailureSignals) ([]byte, error) {
	if signals.IsEmpty() {
		return nil, nil
	}
	return json.Marshal(signals)
}

func unmarshalFailureSignals(data []byte) (FailureSignals, error) {
	if len(data) == 0 {
		return FailureSignals{}, nil
	}
	var signals FailureSignals
	if err := json.Unmarshal(data, &signals); err != nil {
		return FailureSignals{}, err
	}
	return signals, nil
}
