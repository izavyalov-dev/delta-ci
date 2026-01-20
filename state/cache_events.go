package state

import (
	"context"
	"database/sql"
	"errors"
)

type CacheEvent struct {
	ID           int64
	JobAttemptID string
	CacheType    string
	CacheKey     string
	Hit          bool
	ReadOnly     bool
}

func (s *Store) RecordCacheEvents(ctx context.Context, attemptID string, events []CacheEvent) error {
	if attemptID == "" {
		return errors.New("attempt id required")
	}
	if len(events) == 0 {
		return nil
	}

	return s.withTx(ctx, func(tx *sql.Tx) error {
		for _, event := range events {
			cacheType := event.CacheType
			cacheKey := event.CacheKey
			if cacheType == "" || cacheKey == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `
INSERT INTO job_cache_events (job_attempt_id, cache_type, cache_key, cache_hit, read_only)
VALUES ($1, $2, $3, $4, $5)
`, attemptID, cacheType, cacheKey, event.Hit, event.ReadOnly); err != nil {
				return err
			}
		}
		return nil
	})
}
