package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type RunPlan struct {
	RunID         string
	RepoID        string
	Fingerprint   string
	RecipeID      *string
	RecipeSource  string
	RecipeVersion *int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RecipeRecord struct {
	ID          string
	RepoID      string
	Fingerprint string
	Version     int
	Source      string
	RecipeJSON  []byte
	CreatedAt   time.Time
	LastUsedAt  *time.Time
}

func (s *Store) RecordRunPlan(ctx context.Context, plan RunPlan) error {
	if plan.RunID == "" || plan.RepoID == "" {
		return errors.New("run_id and repo_id are required")
	}
	if plan.RecipeSource == "" {
		return errors.New("recipe_source is required")
	}

	var fingerprint sql.NullString
	if plan.Fingerprint != "" {
		fingerprint = sql.NullString{String: plan.Fingerprint, Valid: true}
	}

	var recipeID sql.NullString
	if plan.RecipeID != nil && *plan.RecipeID != "" {
		recipeID = sql.NullString{String: *plan.RecipeID, Valid: true}
	}

	var recipeVersion sql.NullInt64
	if plan.RecipeVersion != nil {
		recipeVersion = sql.NullInt64{Int64: int64(*plan.RecipeVersion), Valid: true}
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO run_plans (run_id, repo_id, fingerprint, recipe_id, recipe_source, recipe_version)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (run_id)
DO UPDATE SET repo_id = EXCLUDED.repo_id,
              fingerprint = EXCLUDED.fingerprint,
              recipe_id = EXCLUDED.recipe_id,
              recipe_source = EXCLUDED.recipe_source,
              recipe_version = EXCLUDED.recipe_version,
              updated_at = NOW()
`, plan.RunID, plan.RepoID, fingerprint, recipeID, plan.RecipeSource, recipeVersion)
	return err
}

func (s *Store) GetRunPlan(ctx context.Context, runID string) (RunPlan, error) {
	if runID == "" {
		return RunPlan{}, errors.New("run_id is required")
	}

	row := s.db.QueryRowContext(ctx, `
SELECT run_id, repo_id, fingerprint, recipe_id, recipe_source, recipe_version, created_at, updated_at
FROM run_plans
WHERE run_id = $1
`, runID)

	var plan RunPlan
	var fingerprint sql.NullString
	var recipeID sql.NullString
	var recipeVersion sql.NullInt64
	if err := row.Scan(&plan.RunID, &plan.RepoID, &fingerprint, &recipeID, &plan.RecipeSource, &recipeVersion, &plan.CreatedAt, &plan.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RunPlan{}, fmt.Errorf("%w: run plan %s", ErrNotFound, runID)
		}
		return RunPlan{}, err
	}
	if fingerprint.Valid {
		plan.Fingerprint = fingerprint.String
	}
	if recipeID.Valid {
		plan.RecipeID = &recipeID.String
	}
	if recipeVersion.Valid {
		value := int(recipeVersion.Int64)
		plan.RecipeVersion = &value
	}
	return plan, nil
}

func (s *Store) CreateRecipe(ctx context.Context, recipe RecipeRecord) (RecipeRecord, bool, error) {
	if recipe.ID == "" {
		return RecipeRecord{}, false, errors.New("recipe id required")
	}
	if recipe.RepoID == "" || recipe.Fingerprint == "" {
		return RecipeRecord{}, false, errors.New("repo_id and fingerprint are required")
	}
	if recipe.Version <= 0 {
		return RecipeRecord{}, false, errors.New("recipe version must be > 0")
	}
	if recipe.Source == "" {
		return RecipeRecord{}, false, errors.New("recipe source required")
	}
	if len(recipe.RecipeJSON) == 0 {
		return RecipeRecord{}, false, errors.New("recipe_json required")
	}

	row := s.db.QueryRowContext(ctx, `
INSERT INTO recipes (id, repo_id, fingerprint, version, source, recipe_json)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (repo_id, fingerprint, version)
DO NOTHING
RETURNING created_at, last_used_at
`, recipe.ID, recipe.RepoID, recipe.Fingerprint, recipe.Version, recipe.Source, recipe.RecipeJSON)

	var createdAt time.Time
	var lastUsedAt sql.NullTime
	if err := row.Scan(&createdAt, &lastUsedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecipeRecord{}, false, nil
		}
		return RecipeRecord{}, false, err
	}
	recipe.CreatedAt = createdAt
	if lastUsedAt.Valid {
		recipe.LastUsedAt = &lastUsedAt.Time
	}
	return recipe, true, nil
}

func (s *Store) FindRecipeByFingerprint(ctx context.Context, repoID, fingerprint string) (RecipeRecord, bool, error) {
	if repoID == "" || fingerprint == "" {
		return RecipeRecord{}, false, errors.New("repo_id and fingerprint are required")
	}

	row := s.db.QueryRowContext(ctx, `
SELECT id, repo_id, fingerprint, version, source, recipe_json, created_at, last_used_at
FROM recipes
WHERE repo_id = $1 AND fingerprint = $2
ORDER BY version DESC, created_at DESC
LIMIT 1
`, repoID, fingerprint)

	var recipe RecipeRecord
	var lastUsedAt sql.NullTime
	if err := row.Scan(&recipe.ID, &recipe.RepoID, &recipe.Fingerprint, &recipe.Version, &recipe.Source, &recipe.RecipeJSON, &recipe.CreatedAt, &lastUsedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RecipeRecord{}, false, nil
		}
		return RecipeRecord{}, false, err
	}
	if lastUsedAt.Valid {
		recipe.LastUsedAt = &lastUsedAt.Time
	}
	return recipe, true, nil
}

func (s *Store) TouchRecipeLastUsed(ctx context.Context, recipeID string, now time.Time) error {
	if recipeID == "" {
		return errors.New("recipe id required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE recipes
SET last_used_at = $2
WHERE id = $1
`, recipeID, now)
	return err
}
