package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
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
	logger     *slog.Logger
	metrics    *observability.Metrics
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
	logger := observability.NewLogger("orchestrator")
	metrics := observability.NewMetrics(nil)
	return &Service{
		store:      store,
		planner:    plan,
		dispatcher: dispatcher,
		ids:        ids,
		logger:     logger,
		metrics:    metrics,
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

	runLogger := observability.WithRun(s.logger, run.ID)
	runLogger.Info("run created", "event", "run_created", "repo_id", run.RepoID, "ref", run.Ref, "commit_sha", run.CommitSHA)
	s.metrics.IncRun("created")

	if err := s.store.TransitionRunState(ctx, runID, state.RunStatePlanning); err != nil {
		return RunDetails{}, err
	}
	runLogger.Info("run planning started", "event", "run_planning")
	s.metrics.IncRun("planning")

	planResult, err := s.planner.Plan(ctx, planner.PlanRequest{
		RunID:     runID,
		RepoID:    req.RepoID,
		Ref:       req.Ref,
		CommitSHA: req.CommitSHA,
	})
	if err != nil {
		_ = s.store.TransitionRunState(ctx, runID, state.RunStatePlanFailed)
		runLogger.Error("planner failed", "event", "plan_failed", "error", err)
		s.metrics.IncRun("plan_failed")
		s.metrics.IncFailure("plan_failed")
		return RunDetails{}, fmt.Errorf("planner failed: %w", err)
	}

	if len(planResult.Jobs) == 0 {
		_ = s.store.TransitionRunState(ctx, runID, state.RunStatePlanFailed)
		runLogger.Error("planner returned no jobs", "event", "plan_failed")
		s.metrics.IncRun("plan_failed")
		s.metrics.IncFailure("plan_failed")
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
		jobLogger := observability.WithJob(runLogger, job.ID)
		jobLogger.Info("job created", "event", "job_created", "name", job.Name, "required", job.Required)
		s.metrics.IncJob("created")

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
		jobLogger.Info("job queued", "event", "job_queued", "attempt_id", attempt.ID)
		s.metrics.IncJob("queued")

		if err := s.store.TransitionJobAttemptState(ctx, attempt.ID, state.JobStateQueued); err != nil {
			return RunDetails{}, err
		}
		attempt.State = state.JobStateQueued

		if err := s.dispatcher.EnqueueJobAttempt(ctx, attempt); err != nil {
			jobLogger.Error("enqueue attempt failed", "event", "enqueue_failed", "error", err)
			s.metrics.IncFailure("enqueue_failed")
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
	runLogger.Info("run queued", "event", "run_queued")
	s.metrics.IncRun("queued")

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

		artifacts, err := s.store.ListArtifactsByJob(ctx, job.ID)
		if err != nil {
			return RunDetails{}, err
		}

		jobDetails = append(jobDetails, JobDetail{
			Job:       job,
			Attempts:  attempts,
			Artifacts: artifacts,
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
	leaseLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, run.ID), job.ID), lease.ID)
	leaseLogger.Info("lease granted", "event", "lease_granted", "attempt_id", attempt.ID, "runner_id", req.RunnerID)
	s.metrics.IncLease("granted")
	s.metrics.IncJob("leased")

	if err := s.store.AckJobAttemptDispatch(ctx, attempt.ID); err != nil && !errors.Is(err, state.ErrNotFound) {
		return protocol.LeaseGranted{}, err
	}

	// Move the run into RUNNING since an attempt is now leased.
	if err := s.store.TransitionRunState(ctx, run.ID, state.RunStateRunning); err != nil {
		return protocol.LeaseGranted{}, err
	}
	s.metrics.IncRun("running")

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
			s.logger.Warn("stale lease ack", "event", "lease_stale", "error", err)
			s.metrics.IncFailure("stale_lease")
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
	ackLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, job.RunID), job.ID), lease.ID)
	ackLogger.Info("lease acknowledged", "event", "lease_acknowledged", "runner_id", msg.RunnerID)
	s.metrics.IncLease("active")
	if attempt.State != state.JobStateStarting {
		s.metrics.IncJob("starting")
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
		s.metrics.IncFailure("stale_lease")
		return protocol.HeartbeatAck{}, ErrStaleLease
	}

	if _, err := s.store.TouchLeaseHeartbeat(ctx, lease.ID, ts); err != nil {
		if state.IsTransitionError(err) {
			s.metrics.IncFailure("stale_lease")
			return protocol.HeartbeatAck{}, ErrStaleLease
		}
		return protocol.HeartbeatAck{}, err
	}

	attempt, err := s.store.GetJobAttempt(ctx, lease.JobAttemptID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}

	transitionedToRunning := attempt.State != state.JobStateRunning
	if err := s.transitionJobAndAttempt(ctx, attempt.JobID, attempt.ID, state.JobStateRunning); err != nil {
		return protocol.HeartbeatAck{}, err
	}

	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}
	heartbeatLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, job.RunID), job.ID), lease.ID)
	heartbeatLogger.Debug("heartbeat received", "event", "lease_heartbeat")
	if transitionedToRunning {
		s.metrics.IncJob("running")
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
		s.metrics.IncFailure("stale_lease")
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
	completeLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, job.RunID), job.ID), lease.ID)

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
	completeLogger.Info("job completed", "event", "job_completed", "status", msg.Status, "exit_code", msg.ExitCode)
	s.metrics.IncJob(jobMetricState(target))
	s.metrics.IncLease("completed")
	if target == state.JobStateFailed {
		s.metrics.IncFailure("job_failed")
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

	if len(msg.Artifacts) > 0 {
		refs := make([]state.ArtifactRef, 0, len(msg.Artifacts))
		for _, artifact := range msg.Artifacts {
			refs = append(refs, state.ArtifactRef{
				Type: artifact.Type,
				URI:  artifact.URI,
			})
		}
		// Artifact references are best-effort; job completion must not be blocked.
		_ = s.store.RecordArtifacts(ctx, attempt.ID, refs)
	}

	return nil
}

func (s *Service) transitionJobAndAttempt(ctx context.Context, jobID, attemptID string, target state.JobState) error {
	if err := s.store.TransitionJobAttemptState(ctx, attemptID, target); err != nil {
		return err
	}
	return s.store.TransitionJobState(ctx, jobID, target)
}

func jobMetricState(stateValue state.JobState) string {
	switch stateValue {
	case state.JobStateSucceeded:
		return "succeeded"
	case state.JobStateFailed:
		return "failed"
	case state.JobStateRunning:
		return "running"
	case state.JobStateQueued:
		return "queued"
	case state.JobStateLeased:
		return "leased"
	case state.JobStateStarting:
		return "starting"
	default:
		return "other"
	}
}
