package state

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrNoExpiredLeases signals there are no leases ready to expire.
var ErrNoExpiredLeases = errors.New("state: no expired leases")

// ExpireLeases finds expired leases and requeues their attempts when allowed.
func (s *Store) ExpireLeases(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	processed := 0
	for processed < limit {
		err := s.withTx(ctx, func(tx *sql.Tx) error {
			var leaseID string
			var attemptID string
			var jobID string
			var leaseState LeaseState
			var attemptState JobState
			var jobState JobState

			row := tx.QueryRowContext(ctx, `
SELECT l.id, l.job_attempt_id, l.state, a.state, a.job_id, j.state
FROM leases l
JOIN job_attempts a ON a.id = l.job_attempt_id
JOIN jobs j ON j.id = a.job_id
WHERE l.state IN ('GRANTED', 'ACTIVE')
  AND l.expires_at IS NOT NULL
  AND l.expires_at <= $1
ORDER BY l.expires_at ASC
FOR UPDATE SKIP LOCKED
LIMIT 1
`, now)

			if err := row.Scan(&leaseID, &attemptID, &leaseState, &attemptState, &jobID, &jobState); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return ErrNoExpiredLeases
				}
				return err
			}

			if err := validateLeaseTransition(leaseID, leaseState, LeaseStateExpired); err != nil {
				return err
			}

			attemptCanQueue := true
			if err := validateJobTransition(attemptID, attemptState, JobStateQueued); err != nil {
				if IsTransitionError(err) {
					attemptCanQueue = false
				} else {
					return err
				}
			}

			jobCanQueue := true
			if err := validateJobTransition(jobID, jobState, JobStateQueued); err != nil {
				if IsTransitionError(err) {
					jobCanQueue = false
				} else {
					return err
				}
			}

			if _, err := tx.ExecContext(ctx, `
UPDATE leases
SET state = $2,
    updated_at = NOW()
WHERE id = $1
`, leaseID, LeaseStateExpired); err != nil {
				return err
			}

			if attemptCanQueue {
				if _, err := tx.ExecContext(ctx, `
UPDATE job_attempts
SET state = $2,
    updated_at = NOW()
WHERE id = $1
`, attemptID, JobStateQueued); err != nil {
					return err
				}
			}

			if jobCanQueue {
				if _, err := tx.ExecContext(ctx, `
UPDATE jobs
SET state = $2,
    updated_at = NOW()
WHERE id = $1
`, jobID, JobStateQueued); err != nil {
					return err
				}
			}

			if attemptCanQueue {
				if _, err := tx.ExecContext(ctx, `
INSERT INTO job_queue (attempt_id, available_at)
VALUES ($1, $2)
ON CONFLICT (attempt_id) DO UPDATE
SET available_at = EXCLUDED.available_at,
    inflight_until = NULL,
    updated_at = NOW()
`, attemptID, now); err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			if errors.Is(err, ErrNoExpiredLeases) {
				break
			}
			return processed, err
		}

		processed++
	}

	return processed, nil
}
