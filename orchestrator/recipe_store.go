package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/state"
)

type recipeStore struct {
	store *state.Store
}

func NewRecipeStore(store *state.Store) planner.RecipeStore {
	if store == nil {
		return nil
	}
	return recipeStore{store: store}
}

func (r recipeStore) FindRecipe(ctx context.Context, repoID, fingerprint string) (planner.Recipe, bool, error) {
	record, ok, err := r.store.FindRecipeByFingerprint(ctx, repoID, fingerprint)
	if err != nil || !ok {
		return planner.Recipe{}, ok, err
	}

	var jobs []planner.PlannedJob
	if err := json.Unmarshal(record.RecipeJSON, &jobs); err != nil {
		return planner.Recipe{}, false, err
	}

	_ = r.store.TouchRecipeLastUsed(ctx, record.ID, time.Now().UTC())

	return planner.Recipe{
		ID:          record.ID,
		Fingerprint: record.Fingerprint,
		Version:     record.Version,
		Source:      record.Source,
		Jobs:        jobs,
	}, true, nil
}
