package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/state"
)

// Service wires planner outputs to state transitions and dispatch.
type Service struct {
	store      *state.Store
	planner    planner.Planner
	dispatcher Dispatcher
	ids        IDGenerator
}

// NewService constructs an orchestrator service with sensible defaults.
func NewService(store *state.Store, plan planner.Planner, dispatcher Dispatcher, ids IDGenerator) *Service {
	if plan == nil {
		plan = planner.StaticPlanner{}
	}
	if dispatcher == nil {
		dispatcher = NoopDispatcher{}
	}
	if ids == nil {
		ids = RandomIDGenerator{}
	}
	return &Service{
		store:      store,
		planner:    plan,
		dispatcher: dispatcher,
		ids:        ids,
	}
}

// CreateRun creates a run, transitions it through planning, creates initial jobs
// and attempts, enqueues them, and returns the resulting state.
func (s *Service) CreateRun(ctx context.Context, req CreateRunRequest) (RunDetails, error) {
	if req.RepoID == "" || req.Ref == "" || req.CommitSHA == "" {
		return RunDetails{}, errors.New("repo_id, ref, and commit_sha are required")
	}

	runID := s.ids.RunID()
	run, err := s.store.CreateRun(ctx, state.Run{
		ID:        runID,
		RepoID:    req.RepoID,
		Ref:       req.Ref,
		CommitSHA: req.CommitSHA,
		State:     state.RunStateCreated,
	})
	if err != nil {
		return RunDetails{}, fmt.Errorf("create run: %w", err)
	}

	if err := s.store.TransitionRunState(ctx, runID, state.RunStatePlanning); err != nil {
		return RunDetails{}, err
	}

	planResult, err := s.planner.Plan(ctx, planner.PlanRequest{
		RunID:     runID,
		RepoID:    req.RepoID,
		Ref:       req.Ref,
		CommitSHA: req.CommitSHA,
	})
	if err != nil {
		_ = s.store.TransitionRunState(ctx, runID, state.RunStatePlanFailed)
		return RunDetails{}, fmt.Errorf("planner failed: %w", err)
	}

	if len(planResult.Jobs) == 0 {
		_ = s.store.TransitionRunState(ctx, runID, state.RunStatePlanFailed)
		return RunDetails{}, errors.New("planner returned no jobs")
	}

	jobDetails := make([]JobDetail, 0, len(planResult.Jobs))
	for _, planned := range planResult.Jobs {
		jobID := s.ids.JobID()
		job, err := s.store.CreateJob(ctx, state.Job{
			ID:       jobID,
			RunID:    runID,
			Name:     planned.Name,
			Required: planned.Required,
			State:    state.JobStateCreated,
		})
		if err != nil {
			return RunDetails{}, fmt.Errorf("create job %s: %w", planned.Name, err)
		}

		attemptID := s.ids.JobAttemptID()
		attempt, err := s.store.CreateJobAttempt(ctx, state.JobAttempt{
			ID:            attemptID,
			JobID:         job.ID,
			AttemptNumber: 1,
			State:         state.JobStateCreated,
		})
		if err != nil {
			return RunDetails{}, fmt.Errorf("create attempt for job %s: %w", job.ID, err)
		}

		if err := s.store.TransitionJobState(ctx, job.ID, state.JobStateQueued); err != nil {
			return RunDetails{}, err
		}
		job.State = state.JobStateQueued
		job.AttemptCount = attempt.AttemptNumber

		if err := s.store.TransitionJobAttemptState(ctx, attempt.ID, state.JobStateQueued); err != nil {
			return RunDetails{}, err
		}
		attempt.State = state.JobStateQueued

		if err := s.dispatcher.EnqueueJobAttempt(ctx, attempt); err != nil {
			return RunDetails{}, fmt.Errorf("enqueue attempt %s: %w", attempt.ID, err)
		}

		jobDetails = append(jobDetails, JobDetail{
			Job:      job,
			Attempts: []state.JobAttempt{attempt},
		})
	}

	if err := s.store.TransitionRunState(ctx, runID, state.RunStateQueued); err != nil {
		return RunDetails{}, err
	}

	// Reload run to capture updated timestamps/state.
	run, err = s.store.GetRun(ctx, runID)
	if err != nil {
		return RunDetails{}, err
	}

	return RunDetails{
		Run:  run,
		Jobs: jobDetails,
	}, nil
}

// GetRunDetails returns read-only run and job data.
func (s *Service) GetRunDetails(ctx context.Context, runID string) (RunDetails, error) {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return RunDetails{}, err
	}

	jobs, err := s.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return RunDetails{}, err
	}

	jobDetails := make([]JobDetail, 0, len(jobs))
	for _, job := range jobs {
		attempts, err := s.store.ListJobAttempts(ctx, job.ID)
		if err != nil {
			return RunDetails{}, err
		}

		jobDetails = append(jobDetails, JobDetail{
			Job:      job,
			Attempts: attempts,
		})
	}

	return RunDetails{
		Run:  run,
		Jobs: jobDetails,
	}, nil
}
