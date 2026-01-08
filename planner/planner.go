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
	Jobs []PlannedJob
}

// PlannedJob describes a single job to schedule.
type PlannedJob struct {
	Name     string
	Required bool
	Spec     protocol.JobSpec
}

// StaticPlanner returns a fixed list of jobs. This keeps Phase 0 simple while
// preserving the planner contract.
type StaticPlanner struct {
	Jobs []PlannedJob
}

func (p StaticPlanner) Plan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	if len(p.Jobs) > 0 {
		return PlanResult{Jobs: p.Jobs}, nil
	}

	// Default to a single required "build" job during early bootstrap.
	return PlanResult{
		Jobs: []PlannedJob{
			{
				Name:     "build",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "build",
					Workdir: ".",
					Steps:   []string{"go build ./..."},
				},
			},
			{
				Name:     "test",
				Required: true,
				Spec: protocol.JobSpec{
					Name:    "test",
					Workdir: ".",
					Steps:   []string{"go test ./..."},
				},
			},
		},
	}, nil
}
