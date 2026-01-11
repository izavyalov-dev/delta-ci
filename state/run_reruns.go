package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrDuplicateRerun indicates a rerun idempotency key already exists.
var ErrDuplicateRerun = errors.New("state: duplicate rerun")

// CreateRunWithRerun creates a run and associates a rerun idempotency key.
func (s *Store) CreateRunWithRerun(ctx context.Context, run Run, originalRunID, idempotencyKey string) (Run, bool, error) {
	if run.State == "" {
		run.State = RunStateCreated
	}
	if originalRunID == "" || idempotencyKey == "" {
		return Run{}, false, errors.New("original_run_id and idempotency_key required")
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
INSERT INTO runs (id, repo_id, ref, commit_sha, state)
VALUES ($1, $2, $3, $4, $5)
RETURNING created_at, updated_at
`, run.ID, run.RepoID, run.Ref, run.CommitSHA, run.State).Scan(&run.CreatedAt, &run.UpdatedAt); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO run_reruns (original_run_id, idempotency_key, new_run_id)
VALUES ($1, $2, $3)
`, originalRunID, idempotencyKey, run.ID); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateRerun
			}
			return err
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateRerun) {
			existing, err := s.getRerunByKey(ctx, originalRunID, idempotencyKey)
			if err != nil {
				return Run{}, false, err
			}
			existingRun, err := s.GetRun(ctx, existing)
			if err != nil {
				return Run{}, false, err
			}
			return existingRun, false, nil
		}
		return Run{}, false, err
	}

	return run, true, nil
}

func (s *Store) getRerunByKey(ctx context.Context, originalRunID, idempotencyKey string) (string, error) {
	var newRunID string
	err := s.db.QueryRowContext(ctx, `
SELECT new_run_id
FROM run_reruns
WHERE original_run_id = $1 AND idempotency_key = $2
`, originalRunID, idempotencyKey).Scan(&newRunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: run rerun %s", ErrNotFound, idempotencyKey)
		}
		return "", err
	}
	return newRunID, nil
}
