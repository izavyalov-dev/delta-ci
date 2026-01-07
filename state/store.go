package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a requested row cannot be located.
var ErrNotFound = errors.New("state: not found")

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// TransitionRunState enforces the documented run state machine using row-level locking.
func (s *Store) TransitionRunState(ctx context.Context, runID string, next RunState) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var current RunState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM runs WHERE id = $1 FOR UPDATE`, runID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: run %s", ErrNotFound, runID)
			}
			return err
		}

		if err := validateRunTransition(runID, current, next); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `UPDATE runs SET state = $2, updated_at = NOW() WHERE id = $1`, runID, next)
		return err
	})
}

// TransitionJobState enforces the documented job state machine using row-level locking.
func (s *Store) TransitionJobState(ctx context.Context, jobID string, next JobState) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var current JobState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: job %s", ErrNotFound, jobID)
			}
			return err
		}

		if err := validateJobTransition(jobID, current, next); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `UPDATE jobs SET state = $2, updated_at = NOW() WHERE id = $1`, jobID, next)
		return err
	})
}

// TransitionJobAttemptState enforces the job attempt state machine using row-level locking.
func (s *Store) TransitionJobAttemptState(ctx context.Context, attemptID string, next JobState) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var current JobState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM job_attempts WHERE id = $1 FOR UPDATE`, attemptID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: job attempt %s", ErrNotFound, attemptID)
			}
			return err
		}

		if err := validateJobTransition(attemptID, current, next); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `UPDATE job_attempts SET state = $2, updated_at = NOW() WHERE id = $1`, attemptID, next)
		return err
	})
}

// TransitionLeaseState enforces the lease state machine using row-level locking.
func (s *Store) TransitionLeaseState(ctx context.Context, leaseID string, next LeaseState) error {
	return s.withTx(ctx, func(tx *sql.Tx) error {
		var current LeaseState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM leases WHERE id = $1 FOR UPDATE`, leaseID).Scan(&current); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: lease %s", ErrNotFound, leaseID)
			}
			return err
		}

		if err := validateLeaseTransition(leaseID, current, next); err != nil {
			return err
		}

		_, err := tx.ExecContext(ctx, `UPDATE leases SET state = $2, updated_at = NOW() WHERE id = $1`, leaseID, next)
		return err
	})
}

func (s *Store) withTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit()
}

// GrantLease atomically creates a lease for the given attempt and moves the attempt/job into LEASED.
func (s *Store) GrantLease(ctx context.Context, attemptID string, lease Lease) (Lease, error) {
	if lease.ID == "" {
		return Lease{}, errors.New("lease id required")
	}
	if lease.TTLSeconds <= 0 {
		return Lease{}, errors.New("ttl_seconds must be > 0")
	}
	if lease.HeartbeatIntervalSeconds <= 0 {
		return Lease{}, errors.New("heartbeat_interval_seconds must be > 0")
	}
	if lease.TTLSeconds <= lease.HeartbeatIntervalSeconds {
		return Lease{}, errors.New("ttl_seconds must be greater than heartbeat_interval_seconds")
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		var jobID string
		var attemptState JobState
		if err := tx.QueryRowContext(ctx, `SELECT job_id, state FROM job_attempts WHERE id = $1 FOR UPDATE`, attemptID).Scan(&jobID, &attemptState); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: job attempt %s", ErrNotFound, attemptID)
			}
			return err
		}

		var jobState JobState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM jobs WHERE id = $1 FOR UPDATE`, jobID).Scan(&jobState); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: job %s", ErrNotFound, jobID)
			}
			return err
		}

		if err := validateJobTransition(attemptID, attemptState, JobStateLeased); err != nil {
			return err
		}
		if err := validateJobTransition(jobID, jobState, JobStateLeased); err != nil {
			return err
		}

		var grantedAt, updatedAt time.Time
		expiresAt := time.Now().Add(time.Duration(lease.TTLSeconds) * time.Second)
		if err := tx.QueryRowContext(ctx, `
INSERT INTO leases (id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING granted_at, updated_at
`, lease.ID, attemptID, lease.RunnerID, LeaseStateGranted, lease.TTLSeconds, lease.HeartbeatIntervalSeconds, expiresAt).Scan(&grantedAt, &updatedAt); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE job_attempts
SET lease_id = $2, state = $3, updated_at = NOW()
WHERE id = $1
`, attemptID, lease.ID, JobStateLeased); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE jobs
SET state = $2, updated_at = NOW()
WHERE id = $1
`, jobID, JobStateLeased); err != nil {
			return err
		}

		lease.JobAttemptID = attemptID
		lease.State = LeaseStateGranted
		lease.GrantedAt = grantedAt
		lease.UpdatedAt = updatedAt
		lease.ExpiresAt = &expiresAt
		return nil
	})

	return lease, err
}

// AcknowledgeLease moves a lease to ACTIVE and records runner identity.
func (s *Store) AcknowledgeLease(ctx context.Context, leaseID string, runnerID string, now time.Time) (Lease, error) {
	var lease Lease
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, granted_at, updated_at, acknowledged_at, last_heartbeat_at, expires_at, completed_at
FROM leases
WHERE id = $1
FOR UPDATE
`, leaseID).Scan(
			&lease.ID,
			&lease.JobAttemptID,
			&lease.RunnerID,
			&lease.State,
			&lease.TTLSeconds,
			&lease.HeartbeatIntervalSeconds,
			&lease.GrantedAt,
			&lease.UpdatedAt,
			&lease.AcknowledgedAt,
			&lease.LastHeartbeatAt,
			&lease.ExpiresAt,
			&lease.CompletedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: lease %s", ErrNotFound, leaseID)
			}
			return err
		}

		if lease.ExpiresAt != nil && now.After(*lease.ExpiresAt) {
			return TransitionError{Entity: "lease", ID: leaseID, From: string(lease.State), To: string(LeaseStateExpired)}
		}

		if err := validateLeaseTransition(leaseID, lease.State, LeaseStateActive); err != nil {
			return err
		}

		expiresAt := now.Add(time.Duration(lease.TTLSeconds) * time.Second)
		if _, err := tx.ExecContext(ctx, `
UPDATE leases
SET state = $2,
    runner_id = $3,
    acknowledged_at = $4,
    expires_at = $5,
    updated_at = $6
WHERE id = $1
`, leaseID, LeaseStateActive, runnerID, now, expiresAt, now); err != nil {
			return err
		}

		lease.State = LeaseStateActive
		lease.RunnerID = &runnerID
		lease.AcknowledgedAt = &now
		lease.ExpiresAt = &expiresAt
		lease.UpdatedAt = now
		return nil
	})
	return lease, err
}

// TouchLeaseHeartbeat updates heartbeat metadata for an active lease if it is still valid.
func (s *Store) TouchLeaseHeartbeat(ctx context.Context, leaseID string, heartbeatTime time.Time) (Lease, error) {
	var lease Lease
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, granted_at, updated_at, acknowledged_at, last_heartbeat_at, expires_at, completed_at
FROM leases
WHERE id = $1
FOR UPDATE
`, leaseID).Scan(
			&lease.ID,
			&lease.JobAttemptID,
			&lease.RunnerID,
			&lease.State,
			&lease.TTLSeconds,
			&lease.HeartbeatIntervalSeconds,
			&lease.GrantedAt,
			&lease.UpdatedAt,
			&lease.AcknowledgedAt,
			&lease.LastHeartbeatAt,
			&lease.ExpiresAt,
			&lease.CompletedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: lease %s", ErrNotFound, leaseID)
			}
			return err
		}

		if lease.ExpiresAt != nil && heartbeatTime.After(*lease.ExpiresAt) {
			return TransitionError{Entity: "lease", ID: leaseID, From: string(lease.State), To: string(LeaseStateExpired)}
		}

		if err := validateLeaseTransition(leaseID, lease.State, LeaseStateActive); err != nil {
			return err
		}

		newExpiry := heartbeatTime.Add(time.Duration(lease.TTLSeconds) * time.Second)
		if _, err := tx.ExecContext(ctx, `
UPDATE leases
SET state = $2,
    last_heartbeat_at = $3,
    expires_at = $4,
    updated_at = $5
WHERE id = $1
`, leaseID, LeaseStateActive, heartbeatTime, newExpiry, heartbeatTime); err != nil {
			return err
		}

		lease.State = LeaseStateActive
		lease.LastHeartbeatAt = &heartbeatTime
		lease.ExpiresAt = &newExpiry
		lease.UpdatedAt = heartbeatTime
		return nil
	})

	return lease, err
}

// CompleteLease finalizes a lease as completed or canceled.
func (s *Store) CompleteLease(ctx context.Context, leaseID string, now time.Time, next LeaseState) (Lease, error) {
	var lease Lease
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
SELECT id, job_attempt_id, runner_id, state, ttl_seconds, heartbeat_interval_seconds, granted_at, updated_at, acknowledged_at, last_heartbeat_at, expires_at, completed_at
FROM leases
WHERE id = $1
FOR UPDATE
`, leaseID).Scan(
			&lease.ID,
			&lease.JobAttemptID,
			&lease.RunnerID,
			&lease.State,
			&lease.TTLSeconds,
			&lease.HeartbeatIntervalSeconds,
			&lease.GrantedAt,
			&lease.UpdatedAt,
			&lease.AcknowledgedAt,
			&lease.LastHeartbeatAt,
			&lease.ExpiresAt,
			&lease.CompletedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: lease %s", ErrNotFound, leaseID)
			}
			return err
		}

		if lease.ExpiresAt != nil && now.After(*lease.ExpiresAt) {
			return TransitionError{Entity: "lease", ID: leaseID, From: string(lease.State), To: string(LeaseStateExpired)}
		}

		if err := validateLeaseTransition(leaseID, lease.State, next); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
UPDATE leases
SET state = $2,
    completed_at = $3,
    updated_at = $4
WHERE id = $1
`, leaseID, next, now, now); err != nil {
			return err
		}

		lease.State = next
		lease.CompletedAt = &now
		lease.UpdatedAt = now
		return nil
	})
	return lease, err
}
