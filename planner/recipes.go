package planner

import "context"

type Recipe struct {
	ID          string
	Fingerprint string
	Version     int
	Source      string
	Jobs        []PlannedJob
}

type RecipeStore interface {
	FindRecipe(ctx context.Context, repoID, fingerprint string) (Recipe, bool, error)
}
