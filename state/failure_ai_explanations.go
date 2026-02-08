package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// RecordFailureAIExplanation persists an advisory AI explanation for a job attempt.
func (s *Store) RecordFailureAIExplanation(ctx context.Context, explanation FailureAIExplanation) error {
	if explanation.JobAttemptID == "" {
		return errors.New("job attempt id required")
	}
	if explanation.Provider == "" {
		return errors.New("ai provider required")
	}
	if explanation.PromptVersion == "" {
		return errors.New("prompt version required")
	}
	if explanation.Summary == "" {
		return errors.New("ai summary required")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO job_failure_ai_explanations (job_attempt_id, provider, model, prompt_version, summary, details, latency_ms)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (job_attempt_id)
DO UPDATE SET provider = EXCLUDED.provider,
              model = EXCLUDED.model,
              prompt_version = EXCLUDED.prompt_version,
              summary = EXCLUDED.summary,
              details = EXCLUDED.details,
              latency_ms = EXCLUDED.latency_ms,
              created_at = NOW()
`, explanation.JobAttemptID, explanation.Provider, nullableString(explanation.Model), explanation.PromptVersion, explanation.Summary, nullableString(explanation.Details), nullableInt(explanation.LatencyMS))
	return err
}

// GetFailureAIExplanationByAttempt fetches the AI explanation for a job attempt.
func (s *Store) GetFailureAIExplanationByAttempt(ctx context.Context, attemptID string) (FailureAIExplanation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_attempt_id, provider, model, prompt_version, summary, details, latency_ms, created_at
FROM job_failure_ai_explanations
WHERE job_attempt_id = $1
`, attemptID)

	var explanation FailureAIExplanation
	var model sql.NullString
	var details sql.NullString
	var latency sql.NullInt64
	if err := row.Scan(&explanation.ID, &explanation.JobAttemptID, &explanation.Provider, &model, &explanation.PromptVersion, &explanation.Summary, &details, &latency, &explanation.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FailureAIExplanation{}, fmt.Errorf("%w: ai explanation for attempt %s", ErrNotFound, attemptID)
		}
		return FailureAIExplanation{}, err
	}
	if model.Valid {
		explanation.Model = model.String
	}
	if details.Valid {
		explanation.Details = details.String
	}
	if latency.Valid {
		explanation.LatencyMS = int(latency.Int64)
	}
	return explanation, nil
}

// ListFailureAIExplanationsByJob returns AI explanations for a job.
func (s *Store) ListFailureAIExplanationsByJob(ctx context.Context, jobID string) ([]FailureAIExplanation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT e.id, e.job_attempt_id, e.provider, e.model, e.prompt_version, e.summary, e.details, e.latency_ms, e.created_at
FROM job_failure_ai_explanations e
JOIN job_attempts ja ON ja.id = e.job_attempt_id
WHERE ja.job_id = $1
ORDER BY e.created_at DESC, e.id DESC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var explanations []FailureAIExplanation
	for rows.Next() {
		var explanation FailureAIExplanation
		var model sql.NullString
		var details sql.NullString
		var latency sql.NullInt64
		if err := rows.Scan(&explanation.ID, &explanation.JobAttemptID, &explanation.Provider, &model, &explanation.PromptVersion, &explanation.Summary, &details, &latency, &explanation.CreatedAt); err != nil {
			return nil, err
		}
		if model.Valid {
			explanation.Model = model.String
		}
		if details.Valid {
			explanation.Details = details.String
		}
		if latency.Valid {
			explanation.LatencyMS = int(latency.Int64)
		}
		explanations = append(explanations, explanation)
	}

	return explanations, rows.Err()
}
