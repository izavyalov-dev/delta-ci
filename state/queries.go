package state

import (
	"context"
	"database/sql"
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

// CreateJob inserts a new job in CREATED state unless explicitly provided.
func (s *Store) CreateJob(ctx context.Context, job Job) (Job, error) {
	if job.State == "" {
		job.State = JobStateCreated
	}

	err := s.db.QueryRowContext(ctx, `
INSERT INTO jobs (id, run_id, name, required, state, attempt_count)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING created_at, updated_at
`, job.ID, job.RunID, job.Name, job.Required, job.State, job.AttemptCount).Scan(&job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}

	return job, nil
}

// ListJobsByRun returns all jobs for a given run ordered by creation time.
func (s *Store) ListJobsByRun(ctx context.Context, runID string) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, run_id, name, required, state, attempt_count, created_at, updated_at
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
		if err := rows.Scan(&job.ID, &job.RunID, &job.Name, &job.Required, &job.State, &job.AttemptCount, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
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

// TouchLeaseHeartbeat updates heartbeat metadata for an active lease.
func (s *Store) TouchLeaseHeartbeat(ctx context.Context, leaseID string, heartbeatTime time.Time, newExpiry time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE leases
SET last_heartbeat_at = $2, expires_at = $3, updated_at = NOW()
WHERE id = $1
`, leaseID, heartbeatTime, newExpiry)
	return err
}
