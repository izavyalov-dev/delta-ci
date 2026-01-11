package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrDuplicateTrigger indicates an idempotency key already exists.
var ErrDuplicateTrigger = errors.New("state: duplicate trigger")

// CreateRunWithTrigger creates a run and associates a trigger for idempotency.
func (s *Store) CreateRunWithTrigger(ctx context.Context, run Run, trigger RunTrigger) (Run, bool, error) {
	if run.State == "" {
		run.State = RunStateCreated
	}
	if trigger.Provider == "" || trigger.EventKey == "" {
		return Run{}, false, errors.New("trigger provider and event_key are required")
	}
	if trigger.EventType == "" || trigger.RepoID == "" || trigger.RepoOwner == "" || trigger.RepoName == "" {
		return Run{}, false, errors.New("trigger event_type and repo metadata required")
	}

	err := s.withTx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `
INSERT INTO runs (id, repo_id, ref, commit_sha, state)
VALUES ($1, $2, $3, $4, $5)
RETURNING created_at, updated_at
`, run.ID, run.RepoID, run.Ref, run.CommitSHA, run.State).Scan(&run.CreatedAt, &run.UpdatedAt); err != nil {
			return err
		}

		var prNumber any
		if trigger.PRNumber != nil {
			prNumber = *trigger.PRNumber
		}

		if _, err := tx.ExecContext(ctx, `
INSERT INTO run_triggers (run_id, provider, event_key, event_type, repo_id, repo_owner, repo_name, pr_number)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, run.ID, trigger.Provider, trigger.EventKey, trigger.EventType, trigger.RepoID, trigger.RepoOwner, trigger.RepoName, prNumber); err != nil {
			if isUniqueViolation(err) {
				return ErrDuplicateTrigger
			}
			return err
		}

		trigger.RunID = run.ID
		return nil
	})

	if err != nil {
		if errors.Is(err, ErrDuplicateTrigger) {
			existingRun, err := s.getRunByTriggerKey(ctx, trigger.Provider, trigger.EventKey)
			if err != nil {
				return Run{}, false, err
			}
			return existingRun, false, nil
		}
		return Run{}, false, err
	}

	return run, true, nil
}

// GetRunTrigger returns trigger metadata for a run.
func (s *Store) GetRunTrigger(ctx context.Context, runID string) (RunTrigger, error) {
	var trigger RunTrigger
	var prNumber sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
SELECT run_id, provider, event_key, event_type, repo_id, repo_owner, repo_name, pr_number, created_at
FROM run_triggers
WHERE run_id = $1
`, runID).Scan(
		&trigger.RunID,
		&trigger.Provider,
		&trigger.EventKey,
		&trigger.EventType,
		&trigger.RepoID,
		&trigger.RepoOwner,
		&trigger.RepoName,
		&prNumber,
		&trigger.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunTrigger{}, fmt.Errorf("%w: run trigger %s", ErrNotFound, runID)
		}
		return RunTrigger{}, err
	}
	if prNumber.Valid {
		value := int(prNumber.Int64)
		trigger.PRNumber = &value
	}
	return trigger, nil
}

func (s *Store) getRunByTriggerKey(ctx context.Context, provider, eventKey string) (Run, error) {
	if provider == "" || eventKey == "" {
		return Run{}, errors.New("provider and event_key required")
	}

	var runID string
	err := s.db.QueryRowContext(ctx, `
SELECT run_id
FROM run_triggers
WHERE provider = $1 AND event_key = $2
`, provider, eventKey).Scan(&runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, fmt.Errorf("%w: run trigger %s", ErrNotFound, eventKey)
		}
		return Run{}, err
	}

	return s.GetRun(ctx, runID)
}
