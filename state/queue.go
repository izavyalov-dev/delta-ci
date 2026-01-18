package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrQueueEmpty indicates that no queue items are available for dispatch.
var ErrQueueEmpty = errors.New("state: queue empty")

// EnqueueJobAttempt publishes a job attempt to the dispatch queue.
func (s *Store) EnqueueJobAttempt(ctx context.Context, attemptID string, availableAt time.Time) error {
	if attemptID == "" {
		return errors.New("attempt id required")
	}
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO job_queue (attempt_id, available_at)
VALUES ($1, $2)
ON CONFLICT (attempt_id)
DO UPDATE SET available_at = EXCLUDED.available_at,
              inflight_until = NULL,
              updated_at = NOW()
`, attemptID, availableAt)
	return err
}

// DequeueJobAttempt returns the next available attempt ID and bumps its visibility window.
func (s *Store) DequeueJobAttempt(ctx context.Context, now time.Time, visibilityTimeout time.Duration) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if visibilityTimeout <= 0 {
		visibilityTimeout = 30 * time.Second
	}

	if _, err := s.db.ExecContext(ctx, `
DELETE FROM job_queue q
USING job_attempts a
JOIN jobs j ON j.id = a.job_id
JOIN runs r ON r.id = j.run_id
WHERE q.attempt_id = a.id
  AND (
    a.state <> 'QUEUED'
    OR r.state IN ('SUCCESS', 'FAILED', 'CANCELED', 'TIMEOUT', 'REPORTED', 'PLAN_FAILED', 'CANCEL_REQUESTED')
  )
`); err != nil {
		return "", err
	}

	var attemptID string
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `
SELECT q.attempt_id
FROM job_queue q
JOIN job_attempts a ON a.id = q.attempt_id
JOIN jobs j ON j.id = a.job_id
JOIN runs r ON r.id = j.run_id
WHERE q.available_at <= $1
  AND (q.inflight_until IS NULL OR q.inflight_until <= $1)
  AND a.state = 'QUEUED'
  AND r.state NOT IN ('SUCCESS', 'FAILED', 'CANCELED', 'TIMEOUT', 'REPORTED', 'PLAN_FAILED', 'CANCEL_REQUESTED')
ORDER BY q.available_at ASC, q.attempt_id ASC
FOR UPDATE SKIP LOCKED
LIMIT 1
`, now)

		if err := row.Scan(&attemptID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrQueueEmpty
			}
			return err
		}

		inflightUntil := now.Add(visibilityTimeout)
		if _, err := tx.ExecContext(ctx, `
UPDATE job_queue
SET inflight_until = $2,
    delivery_count = delivery_count + 1,
    last_delivered_at = $3,
    updated_at = NOW()
WHERE attempt_id = $1
`, attemptID, inflightUntil, now); err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	return attemptID, nil
}

// AckJobAttemptDispatch removes a job attempt from the dispatch queue.
func (s *Store) AckJobAttemptDispatch(ctx context.Context, attemptID string) error {
	if attemptID == "" {
		return errors.New("attempt id required")
	}

	result, err := s.db.ExecContext(ctx, `
DELETE FROM job_queue
WHERE attempt_id = $1
`, attemptID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: queue item %s", ErrNotFound, attemptID)
	}
	return nil
}
