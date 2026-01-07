package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/izavyalov-dev/delta-ci/planner"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

var (
	// ErrStaleLease indicates the lease is not active or has expired.
	ErrStaleLease = errors.New("stale lease")
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

// GrantLease creates a lease for a queued attempt and returns the LeaseGranted payload.
func (s *Service) GrantLease(ctx context.Context, req GrantLeaseRequest) (protocol.LeaseGranted, error) {
	if req.AttemptID == "" {
		return protocol.LeaseGranted{}, errors.New("attempt_id is required")
	}
	if req.TTLSeconds == 0 {
		req.TTLSeconds = 120
	}
	if req.HeartbeatSeconds == 0 {
		req.HeartbeatSeconds = 30
	}
	if req.TTLSeconds <= req.HeartbeatSeconds {
		return protocol.LeaseGranted{}, errors.New("ttl_seconds must exceed heartbeat_seconds")
	}

	attempt, err := s.store.GetJobAttempt(ctx, req.AttemptID)
	if err != nil {
		return protocol.LeaseGranted{}, err
	}

	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return protocol.LeaseGranted{}, err
	}

	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		return protocol.LeaseGranted{}, err
	}

	var runnerIDPtr *string
	if req.RunnerID != "" {
		runnerIDPtr = &req.RunnerID
	}

	leaseID := s.ids.LeaseID()
	lease, err := s.store.GrantLease(ctx, attempt.ID, state.Lease{
		ID:                       leaseID,
		JobAttemptID:             attempt.ID,
		RunnerID:                 runnerIDPtr,
		TTLSeconds:               req.TTLSeconds,
		HeartbeatIntervalSeconds: req.HeartbeatSeconds,
	})
	if err != nil {
		return protocol.LeaseGranted{}, err
	}

	if err := s.store.AckJobAttemptDispatch(ctx, attempt.ID); err != nil && !errors.Is(err, state.ErrNotFound) {
		return protocol.LeaseGranted{}, err
	}

	// Move the run into RUNNING since an attempt is now leased.
	if err := s.store.TransitionRunState(ctx, run.ID, state.RunStateRunning); err != nil {
		return protocol.LeaseGranted{}, err
	}

	spec := protocol.JobSpec{
		Name:    job.Name,
		Workdir: ".",
		Steps:   []string{"echo \"TODO: implement job spec\""},
		Env:     map[string]string{"CI": "true"},
	}

	return protocol.LeaseGranted{
		Type:                     "LeaseGranted",
		RunID:                    run.ID,
		JobID:                    job.ID,
		LeaseID:                  lease.ID,
		LeaseTTLSeconds:          lease.TTLSeconds,
		HeartbeatIntervalSeconds: lease.HeartbeatIntervalSeconds,
		MaxRuntimeSeconds:        req.MaxRuntimeSeconds,
		JobSpec:                  spec,
	}, nil
}

// DequeueJobAttempt pulls the next available job attempt from the queue.
func (s *Service) DequeueJobAttempt(ctx context.Context, visibilityTimeout time.Duration) (string, error) {
	return s.store.DequeueJobAttempt(ctx, time.Now().UTC(), visibilityTimeout)
}

// AckLease transitions an active lease to ACTIVE and moves attempt/job into STARTING.
func (s *Service) AckLease(ctx context.Context, msg protocol.AckLease) error {
	now := msg.AcceptedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	lease, err := s.store.AcknowledgeLease(ctx, msg.LeaseID, msg.RunnerID, now)
	if err != nil {
		if state.IsTransitionError(err) {
			return ErrStaleLease
		}
		return err
	}

	attempt, err := s.store.GetJobAttempt(ctx, lease.JobAttemptID)
	if err != nil {
		return err
	}

	if err := s.transitionJobAndAttempt(ctx, attempt.JobID, attempt.ID, state.JobStateStarting); err != nil {
		return err
	}

	if err := s.store.MarkJobAttemptStarted(ctx, attempt.ID, now); err != nil {
		return err
	}

	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return err
	}
	return s.store.TransitionRunState(ctx, job.RunID, state.RunStateRunning)
}

// HandleHeartbeat updates lease liveness and ensures attempt/job are RUNNING.
func (s *Service) HandleHeartbeat(ctx context.Context, msg protocol.Heartbeat) (protocol.HeartbeatAck, error) {
	ts := msg.TS
	if ts.IsZero() {
		ts = time.Now().UTC()
	}

	lease, err := s.store.GetLease(ctx, msg.LeaseID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}
	if lease.State != state.LeaseStateActive || lease.ExpiresAt != nil && ts.After(*lease.ExpiresAt) {
		return protocol.HeartbeatAck{}, ErrStaleLease
	}

	if _, err := s.store.TouchLeaseHeartbeat(ctx, lease.ID, ts); err != nil {
		if state.IsTransitionError(err) {
			return protocol.HeartbeatAck{}, ErrStaleLease
		}
		return protocol.HeartbeatAck{}, err
	}

	attempt, err := s.store.GetJobAttempt(ctx, lease.JobAttemptID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}

	if err := s.transitionJobAndAttempt(ctx, attempt.JobID, attempt.ID, state.JobStateRunning); err != nil {
		return protocol.HeartbeatAck{}, err
	}

	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}
	if err := s.store.TransitionRunState(ctx, job.RunID, state.RunStateRunning); err != nil {
		return protocol.HeartbeatAck{}, err
	}

	return protocol.HeartbeatAck{
		Type:                  "HeartbeatAck",
		LeaseID:               lease.ID,
		ExtendLease:           true,
		NewLeaseTTLSeconds:    lease.TTLSeconds,
		CancelRequested:       false,
		CancelDeadlineSeconds: 0,
	}, nil
}

// CompleteLease finalizes an attempt for an active lease.
func (s *Service) CompleteLease(ctx context.Context, msg protocol.Complete) error {
	now := msg.FinishedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	lease, err := s.store.GetLease(ctx, msg.LeaseID)
	if err != nil {
		return err
	}
	if lease.State != state.LeaseStateActive || lease.ExpiresAt != nil && now.After(*lease.ExpiresAt) {
		return ErrStaleLease
	}

	attempt, err := s.store.GetJobAttempt(ctx, lease.JobAttemptID)
	if err != nil {
		return err
	}
	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return err
	}

	// Ensure attempt/job are running before uploading -> terminal state.
	if attempt.State != state.JobStateRunning && attempt.State != state.JobStateUploading {
		if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateRunning); err != nil {
			return err
		}
	}

	if attempt.State != state.JobStateUploading {
		if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateUploading); err != nil {
			return err
		}
	}

	var target state.JobState
	switch msg.Status {
	case protocol.CompleteStatusSucceeded:
		target = state.JobStateSucceeded
	case protocol.CompleteStatusFailed:
		target = state.JobStateFailed
	default:
		return fmt.Errorf("unknown completion status %q", msg.Status)
	}

	if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, target); err != nil {
		return err
	}

	if err := s.store.MarkJobAttemptCompleted(ctx, attempt.ID, now); err != nil {
		return err
	}

	if _, err := s.store.CompleteLease(ctx, lease.ID, now, state.LeaseStateCompleted); err != nil {
		if state.IsTransitionError(err) {
			return ErrStaleLease
		}
		return err
	}

	return nil
}

func (s *Service) transitionJobAndAttempt(ctx context.Context, jobID, attemptID string, target state.JobState) error {
	if err := s.store.TransitionJobAttemptState(ctx, attemptID, target); err != nil {
		return err
	}
	return s.store.TransitionJobState(ctx, jobID, target)
}
