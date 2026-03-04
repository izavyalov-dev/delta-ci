package planner

import (
	"context"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

// Planner produces a list of jobs to run for a given run.
type Planner interface {
	Plan(ctx context.Context, req PlanRequest) (PlanResult, error)
}

// PlanRequest contains the context needed to generate a plan.
type PlanRequest struct {
	RunID     string
	RepoID    string
	Ref       string
	CommitSHA string
}

// PlanResult is the outcome of the planning step.
type PlanResult struct {
	Jobs            []PlannedJob
	Explain         string
	SkippedJobs     []SkippedJob
	Fingerprint     string
	RecipeSource    string
	RecipeID        string
	RecipeVersion   int
	DetectedPlugins []string
}

// SkippedJob describes a planned job that was intentionally not scheduled.
type SkippedJob struct {
	Name   string
	Reason string
}

// PlannedJob describes a single job to schedule.
type PlannedJob struct {
	Name      string
	Required  bool
	Spec      protocol.JobSpec
	Reason    string
	DependsOn []string
}

const (
	PlanSourceConfig    = "config"
	PlanSourceRecipe    = "recipe"
	PlanSourceDiscovery = "discovery"
	PlanSourceFallback  = "fallback"
)

// StaticPlanner returns a fixed list of jobs. This keeps Phase 0 simple while
// preserving the planner contract.
type StaticPlanner struct {
	Jobs []PlannedJob
}

func (p StaticPlanner) Plan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	if len(p.Jobs) > 0 {
		return PlanResult{
			Jobs:    p.Jobs,
			Explain: "static planner configured",
		}, nil
	}

	// Polyglot fallback: auto-detect language at runner time and run appropriate commands.
	return PlanResult{
		Explain:      "polyglot fallback (no diff analysis available; language detected at runtime)",
		RecipeSource: PlanSourceFallback,
		Jobs: []PlannedJob{
			{
				Name:     "build",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "build",
					Workdir: ".",
					Steps:   []string{polyglotBuildStep()},
				},
				Reason: "fallback build job (auto-detect language)",
			},
			{
				Name:     "test",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "test",
					Workdir: ".",
					Steps:   []string{polyglotTestStep()},
				},
				Reason:    "fallback test job (auto-detect language)",
				DependsOn: []string{"build"},
			},
			{
				Name:     "lint",
				Required: false,
				Spec: protocol.JobSpec{
					Name:    "lint",
					Workdir: ".",
					Steps:   []string{polyglotLintStep()},
				},
				Reason:    "fallback lint job (auto-detect language)",
				DependsOn: []string{"build"},
			},
		},
	}, nil
}

// polyglotBuildStep returns a shell script that detects the project language and runs
// the appropriate build/install command. Used as a fallback when the planner cannot
// inspect the repository at plan time (e.g. manually-triggered remote runs).
func polyglotBuildStep() string {
	return `set -e
if [ -f go.mod ]; then
  go build ./...
elif [ -f package.json ]; then
  npm install && npm run build --if-present
elif [ -f requirements.txt ]; then
  pip install -r requirements.txt
elif [ -f pyproject.toml ]; then
  pip install -e .
elif find . -maxdepth 3 -name "*.sln" -o -name "*.csproj" 2>/dev/null | grep -q .; then
  dotnet build
else
  echo "No recognized build system detected" && exit 1
fi`
}

func polyglotTestStep() string {
	return `set -e
if [ -f go.mod ]; then
  go test ./...
elif [ -f package.json ]; then
  npm run test --if-present
elif [ -f requirements.txt ] || [ -f pyproject.toml ]; then
  python -m pytest --tb=short 2>/dev/null || python -m unittest discover
elif find . -maxdepth 3 -name "*.sln" -o -name "*.csproj" 2>/dev/null | grep -q .; then
  dotnet test
else
  echo "No recognized test framework detected" && exit 1
fi`
}

func polyglotLintStep() string {
	return `if [ -f go.mod ]; then
  go vet ./...
elif [ -f package.json ]; then
  npm run lint --if-present
elif [ -f requirements.txt ] || [ -f pyproject.toml ]; then
  python -m ruff check . 2>/dev/null || python -m flake8 . 2>/dev/null || true
elif find . -maxdepth 3 -name "*.sln" -o -name "*.csproj" 2>/dev/null | grep -q .; then
  dotnet format --verify-no-changes 2>/dev/null || true
fi`
}
