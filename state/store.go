package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
