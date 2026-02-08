package state

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// RecordFixSuggestion persists an advisory fix suggestion patch.
func (s *Store) RecordFixSuggestion(ctx context.Context, suggestion FixSuggestion) (FixSuggestion, error) {
	if suggestion.JobAttemptID == "" {
		return FixSuggestion{}, errors.New("job attempt id required")
	}
	if suggestion.Provider == "" {
		return FixSuggestion{}, errors.New("ai provider required")
	}
	if suggestion.PromptVersion == "" {
		return FixSuggestion{}, errors.New("prompt version required")
	}
	suggestion.Title = strings.TrimSpace(suggestion.Title)
	if suggestion.Title == "" {
		return FixSuggestion{}, errors.New("fix suggestion title required")
	}
	suggestion.Summary = strings.TrimSpace(suggestion.Summary)
	if suggestion.Summary == "" {
		return FixSuggestion{}, errors.New("fix suggestion summary required")
	}
	if suggestion.PatchFormat == "" {
		suggestion.PatchFormat = FixSuggestionPatchFormatUnifiedDiff
	}
	if err := validateFixSuggestionPatch(suggestion.PatchFormat, suggestion.PatchUnifiedDiff); err != nil {
		return FixSuggestion{}, err
	}
	if !suggestion.RequiresApproval {
		suggestion.RequiresApproval = true
	}
	if suggestion.ValidationStatus == "" {
		suggestion.ValidationStatus = FixSuggestionValidationPending
	}
	suggestion.PatchSHA256 = hashPatch(suggestion.PatchUnifiedDiff)

	row := s.db.QueryRowContext(ctx, `
INSERT INTO job_fix_suggestions (
	job_attempt_id, provider, model, prompt_version, title, summary, patch_format, patch_unified_diff,
	patch_sha256, validation_run_id, validation_job_id, validation_status, validation_summary, requires_approval
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (job_attempt_id, patch_sha256)
DO UPDATE SET provider = EXCLUDED.provider,
              model = EXCLUDED.model,
              prompt_version = EXCLUDED.prompt_version,
              title = EXCLUDED.title,
              summary = EXCLUDED.summary,
              patch_format = EXCLUDED.patch_format,
              patch_unified_diff = EXCLUDED.patch_unified_diff,
              validation_run_id = EXCLUDED.validation_run_id,
              validation_job_id = EXCLUDED.validation_job_id,
              validation_status = EXCLUDED.validation_status,
              validation_summary = EXCLUDED.validation_summary,
              requires_approval = EXCLUDED.requires_approval,
              updated_at = NOW()
RETURNING id, created_at, updated_at
`, suggestion.JobAttemptID, suggestion.Provider, nullableString(suggestion.Model), suggestion.PromptVersion, suggestion.Title, suggestion.Summary, suggestion.PatchFormat, suggestion.PatchUnifiedDiff, suggestion.PatchSHA256, suggestion.ValidationRunID, suggestion.ValidationJobID, suggestion.ValidationStatus, nullableString(suggestion.ValidationSummary), suggestion.RequiresApproval)
	if err := row.Scan(&suggestion.ID, &suggestion.CreatedAt, &suggestion.UpdatedAt); err != nil {
		return FixSuggestion{}, err
	}
	return suggestion, nil
}

// GetFixSuggestion returns a fix suggestion by ID.
func (s *Store) GetFixSuggestion(ctx context.Context, suggestionID int64) (FixSuggestion, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_attempt_id, provider, model, prompt_version, title, summary, patch_format, patch_unified_diff, patch_sha256,
       validation_run_id, validation_job_id, validation_status, validation_summary, requires_approval, created_at, updated_at
FROM job_fix_suggestions
WHERE id = $1
`, suggestionID)
	return scanFixSuggestion(row)
}

// ListFixSuggestionsByJob returns fix suggestions associated with a job.
func (s *Store) ListFixSuggestionsByJob(ctx context.Context, jobID string) ([]FixSuggestion, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT s.id, s.job_attempt_id, s.provider, s.model, s.prompt_version, s.title, s.summary, s.patch_format, s.patch_unified_diff, s.patch_sha256,
       s.validation_run_id, s.validation_job_id, s.validation_status, s.validation_summary, s.requires_approval, s.created_at, s.updated_at
FROM job_fix_suggestions s
JOIN job_attempts ja ON ja.id = s.job_attempt_id
WHERE ja.job_id = $1
ORDER BY s.created_at DESC, s.id DESC
`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suggestions := make([]FixSuggestion, 0)
	for rows.Next() {
		suggestion, err := scanFixSuggestion(rows)
		if err != nil {
			return nil, err
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions, rows.Err()
}

// GetFixSuggestionByValidationJob returns the suggestion for a validation job.
func (s *Store) GetFixSuggestionByValidationJob(ctx context.Context, validationJobID string) (FixSuggestion, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, job_attempt_id, provider, model, prompt_version, title, summary, patch_format, patch_unified_diff, patch_sha256,
       validation_run_id, validation_job_id, validation_status, validation_summary, requires_approval, created_at, updated_at
FROM job_fix_suggestions
WHERE validation_job_id = $1
`, validationJobID)
	return scanFixSuggestion(row)
}

// AttachFixSuggestionValidation links a suggestion to a validation run/job and status.
func (s *Store) AttachFixSuggestionValidation(ctx context.Context, suggestionID int64, runID, jobID string) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE job_fix_suggestions
SET validation_run_id = $2,
    validation_job_id = $3,
    validation_status = $4,
    validation_summary = NULL,
    updated_at = NOW()
WHERE id = $1
`, suggestionID, nullableString(runID), nullableString(jobID), FixSuggestionValidationQueued)
	return err
}

// UpdateFixSuggestionValidationByJob sets validation status/result for a mapped job.
func (s *Store) UpdateFixSuggestionValidationByJob(ctx context.Context, validationJobID string, status FixSuggestionValidationStatus, summary string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE job_fix_suggestions
SET validation_status = $2,
    validation_summary = $3,
    updated_at = NOW()
WHERE validation_job_id = $1
`, validationJobID, status, nullableString(strings.TrimSpace(summary)))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: fix suggestion for validation job %s", ErrNotFound, validationJobID)
	}
	return nil
}

func validateFixSuggestionPatch(format FixSuggestionPatchFormat, patch string) error {
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return errors.New("patch content required")
	}
	switch format {
	case FixSuggestionPatchFormatUnifiedDiff:
		if !strings.Contains(patch, "\n--- ") && !strings.HasPrefix(patch, "--- ") {
			return errors.New("unified diff patch must include --- header")
		}
		if !strings.Contains(patch, "\n+++ ") && !strings.Contains(patch, "\n+++ b/") {
			return errors.New("unified diff patch must include +++ header")
		}
		if !strings.Contains(patch, "@@") {
			return errors.New("unified diff patch must include hunk markers")
		}
	default:
		return fmt.Errorf("unsupported patch format %q", format)
	}
	return nil
}

func hashPatch(patch string) string {
	sum := sha256.Sum256([]byte(patch))
	return hex.EncodeToString(sum[:])
}

type scanRow interface {
	Scan(dest ...any) error
}

func scanFixSuggestion(row scanRow) (FixSuggestion, error) {
	var suggestion FixSuggestion
	var model sql.NullString
	var validationRunID sql.NullString
	var validationJobID sql.NullString
	var validationSummary sql.NullString
	if err := row.Scan(
		&suggestion.ID,
		&suggestion.JobAttemptID,
		&suggestion.Provider,
		&model,
		&suggestion.PromptVersion,
		&suggestion.Title,
		&suggestion.Summary,
		&suggestion.PatchFormat,
		&suggestion.PatchUnifiedDiff,
		&suggestion.PatchSHA256,
		&validationRunID,
		&validationJobID,
		&suggestion.ValidationStatus,
		&validationSummary,
		&suggestion.RequiresApproval,
		&suggestion.CreatedAt,
		&suggestion.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return FixSuggestion{}, fmt.Errorf("%w: fix suggestion", ErrNotFound)
		}
		return FixSuggestion{}, err
	}
	if model.Valid {
		suggestion.Model = model.String
	}
	if validationRunID.Valid {
		suggestion.ValidationRunID = &validationRunID.String
	}
	if validationJobID.Valid {
		suggestion.ValidationJobID = &validationJobID.String
	}
	if validationSummary.Valid {
		suggestion.ValidationSummary = validationSummary.String
	}
	return suggestion, nil
}
