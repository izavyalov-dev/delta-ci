package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetStatusReport returns reporting metadata for a run/provider.
func (s *Store) GetStatusReport(ctx context.Context, runID, provider string) (StatusReport, error) {
	var report StatusReport
	var checkRunID sql.NullString
	var prCommentID sql.NullString
	err := s.db.QueryRowContext(ctx, `
SELECT run_id, provider, check_run_id, pr_comment_id, last_state, created_at, updated_at
FROM vcs_status_reports
WHERE run_id = $1 AND provider = $2
`, runID, provider).Scan(
		&report.RunID,
		&report.Provider,
		&checkRunID,
		&prCommentID,
		&report.LastState,
		&report.CreatedAt,
		&report.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StatusReport{}, fmt.Errorf("%w: status report %s", ErrNotFound, runID)
		}
		return StatusReport{}, err
	}
	if checkRunID.Valid {
		report.CheckRunID = &checkRunID.String
	}
	if prCommentID.Valid {
		report.PRCommentID = &prCommentID.String
	}
	return report, nil
}

// UpsertStatusReport records reporting metadata for a run/provider.
func (s *Store) UpsertStatusReport(ctx context.Context, report StatusReport) (StatusReport, error) {
	if report.RunID == "" || report.Provider == "" {
		return StatusReport{}, errors.New("run_id and provider required")
	}

	var checkRunID any
	if report.CheckRunID != nil {
		checkRunID = *report.CheckRunID
	}
	var prCommentID any
	if report.PRCommentID != nil {
		prCommentID = *report.PRCommentID
	}

	var stored StatusReport
	var storedCheck sql.NullString
	var storedComment sql.NullString
	err := s.db.QueryRowContext(ctx, `
INSERT INTO vcs_status_reports (run_id, provider, check_run_id, pr_comment_id, last_state)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (run_id, provider)
DO UPDATE SET
	check_run_id = COALESCE(EXCLUDED.check_run_id, vcs_status_reports.check_run_id),
	pr_comment_id = COALESCE(EXCLUDED.pr_comment_id, vcs_status_reports.pr_comment_id),
	last_state = EXCLUDED.last_state,
	updated_at = NOW()
RETURNING run_id, provider, check_run_id, pr_comment_id, last_state, created_at, updated_at
`, report.RunID, report.Provider, checkRunID, prCommentID, report.LastState).Scan(
		&stored.RunID,
		&stored.Provider,
		&storedCheck,
		&storedComment,
		&stored.LastState,
		&stored.CreatedAt,
		&stored.UpdatedAt,
	)
	if err != nil {
		return StatusReport{}, err
	}
	if storedCheck.Valid {
		stored.CheckRunID = &storedCheck.String
	}
	if storedComment.Valid {
		stored.PRCommentID = &storedComment.String
	}
	return stored, nil
}
