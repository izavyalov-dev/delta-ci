package state

import (
	"context"
	"database/sql"
	"time"
)

// DeadLetterExpired moves queue items exceeding maxDeliveries to the dead_letter_log
// table and transitions their attempts to FAILED. Returns the number of items processed.
func (s *Store) DeadLetterExpired(ctx context.Context, maxDeliveries, limit int) (int, error) {
	if maxDeliveries <= 0 {
		maxDeliveries = 5
	}
	if limit <= 0 {
		limit = 50
	}

	type deadItem struct {
		attemptID     string
		deliveryCount int
	}

	var items []deadItem
	err := s.withTx(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `
SELECT q.attempt_id, q.delivery_count
FROM job_queue q
WHERE q.delivery_count >= $1
ORDER BY q.updated_at ASC
LIMIT $2
FOR UPDATE SKIP LOCKED
`, maxDeliveries, limit)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item deadItem
			if err := rows.Scan(&item.attemptID, &item.deliveryCount); err != nil {
				return err
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		now := time.Now().UTC()
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `
INSERT INTO dead_letter_log (attempt_id, delivery_count, reason, dead_lettered_at)
VALUES ($1, $2, 'max_deliveries_exceeded', $3)
`, item.attemptID, item.deliveryCount, now); err != nil {
				return err
			}

			if _, err := tx.ExecContext(ctx, `DELETE FROM job_queue WHERE attempt_id = $1`, item.attemptID); err != nil {
				return err
			}

			// Transition attempt to FAILED.
			if _, err := tx.ExecContext(ctx, `
UPDATE job_attempts SET state = 'FAILED', completed_at = $2, updated_at = NOW()
WHERE id = $1 AND state = 'QUEUED'
`, item.attemptID, now); err != nil {
				return err
			}

			// Also transition the parent job to FAILED.
			if _, err := tx.ExecContext(ctx, `
UPDATE jobs SET state = 'FAILED', updated_at = NOW()
WHERE id = (SELECT job_id FROM job_attempts WHERE id = $1)
  AND state IN ('CREATED', 'QUEUED')
`, item.attemptID); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return len(items), nil
}
