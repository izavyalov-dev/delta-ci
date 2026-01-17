package orchestrator

import (
	"context"
	"encoding/json"
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
	// ErrInvalidRunState indicates the run cannot accept the requested transition.
	ErrInvalidRunState = errors.New("invalid run state")
)

// Service wires planner outputs to state transitions and dispatch.
type Service struct {
	store      *state.Store
	planner    planner.Planner
	dispatcher Dispatcher
	ids        IDGenerator
	reporter   StatusReporter
	analyzer   FailureAnalyzer
	logger     *slog.Logger
	metrics    *observability.Metrics
}

// NewService constructs an orchestrator service with sensible defaults.
func NewService(store *state.Store, plan planner.Planner, dispatcher Dispatcher, ids IDGenerator, reporter StatusReporter, analyzer FailureAnalyzer) *Service {
	if plan == nil {
		plan = planner.StaticPlanner{}
	}
	if dispatcher == nil {
		dispatcher = NoopDispatcher{}
	}
	if ids == nil {
		ids = RandomIDGenerator{}
	}
	if reporter == nil {
		reporter = NoopStatusReporter{}
	}
	if analyzer == nil {
		analyzer = NewRuleBasedFailureAnalyzer()
	}
	logger := observability.NewLogger("orchestrator")
	metrics := observability.NewMetrics(nil)
	return &Service{
		store:      store,
		planner:    plan,
		dispatcher: dispatcher,
		ids:        ids,
		reporter:   reporter,
		analyzer:   analyzer,
		logger:     logger,
		metrics:    metrics,
	}
}

// CreateRun creates a run, transitions it through planning, creates initial jobs
// and attempts, enqueues them, and returns the resulting state.
func (s *Service) CreateRun(ctx context.Context, req CreateRunRequest) (RunDetails, error) {
	if err := validateCreateRunRequest(req); err != nil {
		return RunDetails{}, err
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

	return s.startRun(ctx, run)
}

// CreateRunFromTrigger creates a run with a webhook trigger if the event is new.
func (s *Service) CreateRunFromTrigger(ctx context.Context, req CreateRunRequest, trigger state.RunTrigger) (RunDetails, bool, error) {
	if err := validateCreateRunRequest(req); err != nil {
		return RunDetails{}, false, err
	}

	runID := s.ids.RunID()
	run, created, err := s.store.CreateRunWithTrigger(ctx, state.Run{
		ID:        runID,
		RepoID:    req.RepoID,
		Ref:       req.Ref,
		CommitSHA: req.CommitSHA,
		State:     state.RunStateCreated,
	}, trigger)
	if err != nil {
		return RunDetails{}, false, err
	}
	if !created {
		details, err := s.GetRunDetails(ctx, run.ID)
		return details, false, err
	}

	details, err := s.startRun(ctx, run)
	return details, true, err
}

// RerunRun creates a new run attempt for an existing run using an idempotency key.
func (s *Service) RerunRun(ctx context.Context, runID, idempotencyKey string) (RunDetails, bool, error) {
	if runID == "" || idempotencyKey == "" {
		return RunDetails{}, false, errors.New("run_id and idempotency_key are required")
	}

	original, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return RunDetails{}, false, err
	}

	newRunID := s.ids.RunID()
	run, created, err := s.store.CreateRunWithRerun(ctx, state.Run{
		ID:        newRunID,
		RepoID:    original.RepoID,
		Ref:       original.Ref,
		CommitSHA: original.CommitSHA,
		State:     state.RunStateCreated,
	}, original.ID, idempotencyKey)
	if err != nil {
		return RunDetails{}, false, err
	}
	if !created {
		details, err := s.GetRunDetails(ctx, run.ID)
		return details, false, err
	}

	details, err := s.startRun(ctx, run)
	return details, true, err
}

// CancelRun transitions a run to cancel requested and propagates to jobs.
func (s *Service) CancelRun(ctx context.Context, runID string) (RunDetails, error) {
	if runID == "" {
		return RunDetails{}, errors.New("run_id is required")
	}

	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return RunDetails{}, err
	}

	switch run.State {
	case state.RunStateCancelRequested, state.RunStateCanceled:
		return s.GetRunDetails(ctx, runID)
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateTimeout, state.RunStateReported:
		return RunDetails{}, fmt.Errorf("%w: run %s is already terminal (%s)", ErrInvalidRunState, runID, run.State)
	}

	if err := s.store.TransitionRunState(ctx, runID, state.RunStateCancelRequested); err != nil {
		return RunDetails{}, err
	}
	s.metrics.IncRun("cancel_requested")
	s.reportRun(ctx, runID)

	jobs, err := s.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return RunDetails{}, err
	}

	now := time.Now().UTC()
	for _, job := range jobs {
		attempt, err := s.store.GetLatestJobAttempt(ctx, job.ID)
		if err != nil {
			return RunDetails{}, err
		}
		switch job.State {
		case state.JobStateQueued:
			if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateCancelRequested); err != nil {
				return RunDetails{}, err
			}
			if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateCanceled); err != nil {
				return RunDetails{}, err
			}
			if err := s.store.MarkJobAttemptCompleted(ctx, attempt.ID, now); err != nil {
				return RunDetails{}, err
			}
			s.metrics.IncJob("canceled")
		case state.JobStateLeased, state.JobStateStarting, state.JobStateRunning:
			if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateCancelRequested); err != nil {
				return RunDetails{}, err
			}
		default:
			continue
		}
	}

	if err := s.finalizeCancelIfReady(ctx, runID); err != nil {
		return RunDetails{}, err
	}

	return s.GetRunDetails(ctx, runID)
}

func validateCreateRunRequest(req CreateRunRequest) error {
	if req.RepoID == "" || req.Ref == "" || req.CommitSHA == "" {
		return errors.New("repo_id, ref, and commit_sha are required")
	}
	return nil
}

func (s *Service) startRun(ctx context.Context, run state.Run) (RunDetails, error) {
	runLogger := observability.WithRun(s.logger, run.ID)
	runLogger.Info("run created", "event", "run_created", "repo_id", run.RepoID, "ref", run.Ref, "commit_sha", run.CommitSHA)
	s.metrics.IncRun("created")

	if err := s.store.TransitionRunState(ctx, run.ID, state.RunStatePlanning); err != nil {
		return RunDetails{}, err
	}
	runLogger.Info("run planning started", "event", "run_planning")
	s.metrics.IncRun("planning")
	s.reportRun(ctx, run.ID)

	planResult, err := s.planner.Plan(ctx, planner.PlanRequest{
		RunID:     run.ID,
		RepoID:    run.RepoID,
		Ref:       run.Ref,
		CommitSHA: run.CommitSHA,
	})
	if err != nil {
		if failErr := s.failRun(ctx, run.ID, runLogger, "plan_failed", err); failErr != nil {
			return RunDetails{}, failErr
		}
		return RunDetails{}, fmt.Errorf("planner failed: %w", err)
	}

	if len(planResult.Jobs) == 0 {
		if failErr := s.failRun(ctx, run.ID, runLogger, "plan_failed", errors.New("planner returned no jobs")); failErr != nil {
			return RunDetails{}, failErr
		}
		return RunDetails{}, errors.New("planner returned no jobs")
	}
	if planResult.Explain != "" {
		runLogger.Info("plan generated", "event", "plan_generated", "explain", planResult.Explain)
	}

	jobDetails := make([]JobDetail, 0, len(planResult.Jobs))
	for _, planned := range planResult.Jobs {
		jobID := s.ids.JobID()
		job, err := s.store.CreateJob(ctx, state.Job{
			ID:       jobID,
			RunID:    run.ID,
			Name:     planned.Name,
			Required: planned.Required,
			State:    state.JobStateCreated,
		})
		if err != nil {
			return RunDetails{}, fmt.Errorf("create job %s: %w", planned.Name, err)
		}
		jobLogger := observability.WithJob(runLogger, job.ID)
		jobFields := []any{"event", "job_created", "name", job.Name, "required", job.Required}
		if planned.Reason != "" {
			jobFields = append(jobFields, "reason", planned.Reason)
		}
		jobLogger.Info("job created", jobFields...)
		s.metrics.IncJob("created")

		spec := planned.Spec
		if spec.Name == "" {
			spec.Name = job.Name
		}
		if spec.Workdir == "" {
			spec.Workdir = "."
		}
		if len(spec.Steps) == 0 {
			spec.Steps = []string{"echo \"job spec missing\""}
		}
		specJSON, err := json.Marshal(spec)
		if err != nil {
			return RunDetails{}, fmt.Errorf("encode job spec %s: %w", job.ID, err)
		}
		if err := s.store.RecordJobSpec(ctx, job.ID, specJSON); err != nil {
			return RunDetails{}, fmt.Errorf("record job spec %s: %w", job.ID, err)
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
			Job:                 job,
			Attempts:            []state.JobAttempt{attempt},
			Artifacts:           nil,
			FailureExplanations: nil,
		})
	}

	if err := s.store.TransitionRunState(ctx, run.ID, state.RunStateQueued); err != nil {
		return RunDetails{}, err
	}
	runLogger.Info("run queued", "event", "run_queued")
	s.metrics.IncRun("queued")
	s.reportRun(ctx, run.ID)

	// Reload run to capture updated timestamps/state.
	run, err = s.store.GetRun(ctx, run.ID)
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

		failureExplanations, err := s.store.ListFailureExplanationsByJob(ctx, job.ID)
		if err != nil {
			return RunDetails{}, err
		}

		jobDetails = append(jobDetails, JobDetail{
			Job:                 job,
			Attempts:            attempts,
			Artifacts:           artifacts,
			FailureExplanations: failureExplanations,
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
	if isRunTerminal(run.State) || run.State == state.RunStateCancelRequested {
		return protocol.LeaseGranted{}, fmt.Errorf("run %s is not leasable (%s)", run.ID, run.State)
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
	s.reportRun(ctx, run.ID)

	specJSON, err := s.store.GetJobSpec(ctx, job.ID)
	if err != nil {
		return protocol.LeaseGranted{}, err
	}

	var spec protocol.JobSpec
	if err := json.Unmarshal(specJSON, &spec); err != nil {
		return protocol.LeaseGranted{}, fmt.Errorf("decode job spec %s: %w", job.ID, err)
	}
	if spec.Name == "" {
		spec.Name = job.Name
	}
	if spec.Workdir == "" {
		spec.Workdir = "."
	}
	if len(spec.Steps) == 0 {
		return protocol.LeaseGranted{}, errors.New("job spec steps required")
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

// ExpireLeases sweeps expired leases and requeues attempts.
func (s *Service) ExpireLeases(ctx context.Context, limit int) (int, error) {
	count, err := s.store.ExpireLeases(ctx, time.Now().UTC(), limit)
	if err != nil {
		return 0, err
	}
	for i := 0; i < count; i++ {
		s.metrics.IncLease("expired")
	}
	if count > 0 {
		s.logger.Info("expired leases requeued", "event", "lease_expired", "count", count)
	}
	return count, nil
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

	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		return err
	}
	if run.State == state.RunStateCancelRequested || isRunTerminal(run.State) {
		return nil
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

	job, err := s.store.GetJob(ctx, attempt.JobID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}

	run, err := s.store.GetRun(ctx, job.RunID)
	if err != nil {
		return protocol.HeartbeatAck{}, err
	}
	cancelRequested := run.State == state.RunStateCancelRequested || job.State == state.JobStateCancelRequested

	transitionedToRunning := attempt.State != state.JobStateRunning
	if attempt.State != state.JobStateCancelRequested {
		if err := s.transitionJobAndAttempt(ctx, attempt.JobID, attempt.ID, state.JobStateRunning); err != nil {
			return protocol.HeartbeatAck{}, err
		}
	} else {
		transitionedToRunning = false
	}
	heartbeatLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, job.RunID), job.ID), lease.ID)
	heartbeatLogger.Debug("heartbeat received", "event", "lease_heartbeat")
	if transitionedToRunning {
		s.metrics.IncJob("running")
	}
	if !cancelRequested && !isRunTerminal(run.State) {
		if err := s.store.TransitionRunState(ctx, job.RunID, state.RunStateRunning); err != nil {
			return protocol.HeartbeatAck{}, err
		}
	}

	deadline := 0
	if cancelRequested {
		deadline = cancelDeadlineSeconds
	}

	return protocol.HeartbeatAck{
		Type:                  "HeartbeatAck",
		LeaseID:               lease.ID,
		ExtendLease:           true,
		NewLeaseTTLSeconds:    lease.TTLSeconds,
		CancelRequested:       cancelRequested,
		CancelDeadlineSeconds: deadline,
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
	run, err := s.store.GetRun(ctx, job.RunID)
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

	var artifactRefs []state.ArtifactRef
	if len(msg.Artifacts) > 0 {
		artifactRefs = make([]state.ArtifactRef, 0, len(msg.Artifacts))
		for _, artifact := range msg.Artifacts {
			artifactRefs = append(artifactRefs, state.ArtifactRef{
				Type: artifact.Type,
				URI:  artifact.URI,
			})
		}
		// Artifact references are best-effort; job completion must not be blocked.
		_ = s.store.RecordArtifacts(ctx, attempt.ID, artifactRefs)
	}

	if target == state.JobStateFailed {
		s.recordFailureExplanation(ctx, job, attempt, msg, artifactRefs)
	}

	if run.State == state.RunStateCancelRequested {
		if err := s.finalizeCancelIfReady(ctx, job.RunID); err != nil {
			s.metrics.IncFailure("run_finalize_failed")
			completeLogger.Error("run finalization failed", "event", "run_finalize_failed", "error", err)
		}
	} else {
		if err := s.finalizeRunIfReady(ctx, job.RunID); err != nil {
			s.metrics.IncFailure("run_finalize_failed")
			completeLogger.Error("run finalization failed", "event", "run_finalize_failed", "error", err)
		}
	}

	return nil
}

// CancelLease finalizes an attempt when a runner acknowledges cancellation.
func (s *Service) CancelLease(ctx context.Context, msg protocol.CancelAck) error {
	now := msg.TS
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if msg.FinalStatus != protocol.CancelFinalStatusCanceled {
		return fmt.Errorf("unsupported cancel status %q", msg.FinalStatus)
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
	if job.State != state.JobStateCancelRequested {
		return ErrStaleLease
	}

	cancelLogger := observability.WithLease(observability.WithJob(observability.WithRun(s.logger, job.RunID), job.ID), lease.ID)
	if err := s.transitionJobAndAttempt(ctx, job.ID, attempt.ID, state.JobStateCanceled); err != nil {
		if state.IsTransitionError(err) {
			return ErrStaleLease
		}
		return err
	}
	s.metrics.IncJob("canceled")
	s.metrics.IncLease("canceled")

	if err := s.store.MarkJobAttemptCompleted(ctx, attempt.ID, now); err != nil {
		return err
	}

	if _, err := s.store.CompleteLease(ctx, lease.ID, now, state.LeaseStateCanceled); err != nil {
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
		_ = s.store.RecordArtifacts(ctx, attempt.ID, refs)
	}

	if err := s.finalizeCancelIfReady(ctx, job.RunID); err != nil {
		s.metrics.IncFailure("run_finalize_failed")
		cancelLogger.Error("run cancel finalization failed", "event", "run_finalize_failed", "error", err)
	}

	cancelLogger.Info("job canceled", "event", "job_canceled")
	return nil
}

func (s *Service) recordFailureExplanation(ctx context.Context, job state.Job, attempt state.JobAttempt, msg protocol.Complete, artifacts []state.ArtifactRef) {
	if s.analyzer == nil {
		return
	}
	explanation, err := s.analyzer.Analyze(ctx, FailureInput{
		RunID:     job.RunID,
		JobID:     job.ID,
		JobName:   job.Name,
		AttemptID: attempt.ID,
		Status:    msg.Status,
		ExitCode:  msg.ExitCode,
		Summary:   msg.Summary,
		Artifacts: artifacts,
	})
	if err != nil {
		s.metrics.IncFailure("failure_analysis_failed")
		s.logger.Error("failure analysis failed", "event", "failure_analysis_failed", "job_id", job.ID, "attempt_id", attempt.ID, "error", err)
		return
	}
	if explanation == nil {
		return
	}
	if err := s.store.RecordFailureExplanation(ctx, *explanation); err != nil {
		s.metrics.IncFailure("failure_analysis_failed")
		s.logger.Error("failure explanation persist failed", "event", "failure_explanation_failed", "job_id", job.ID, "attempt_id", attempt.ID, "error", err)
		return
	}
	s.logger.Info("failure explanation recorded", "event", "failure_explanation_recorded", "job_id", job.ID, "attempt_id", attempt.ID, "category", explanation.Category, "confidence", explanation.Confidence)
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

const cancelDeadlineSeconds = 30

func (s *Service) finalizeRunIfReady(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.State != state.RunStateRunning {
		return nil
	}

	jobs, err := s.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return err
	}

	allRequiredSucceeded := true
	for _, job := range jobs {
		if !job.Required {
			continue
		}
		switch job.State {
		case state.JobStateSucceeded:
			continue
		case state.JobStateFailed, state.JobStateTimedOut, state.JobStateCanceled:
			allRequiredSucceeded = false
			if err := s.store.TransitionRunState(ctx, runID, state.RunStateFailed); err != nil {
				return err
			}
			s.metrics.IncRun("failed")
			s.logger.Info("run failed", "event", "run_failed", "run_id", runID)
			s.reportRun(ctx, runID)
			return nil
		default:
			return nil
		}
	}

	if allRequiredSucceeded {
		if err := s.store.TransitionRunState(ctx, runID, state.RunStateSuccess); err != nil {
			return err
		}
		s.metrics.IncRun("success")
		s.logger.Info("run succeeded", "event", "run_succeeded", "run_id", runID)
		s.reportRun(ctx, runID)
	}

	return nil
}

func (s *Service) finalizeCancelIfReady(ctx context.Context, runID string) error {
	run, err := s.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.State != state.RunStateCancelRequested {
		return nil
	}

	jobs, err := s.store.ListJobsByRun(ctx, runID)
	if err != nil {
		return err
	}

	for _, job := range jobs {
		switch job.State {
		case state.JobStateQueued, state.JobStateLeased, state.JobStateStarting, state.JobStateRunning, state.JobStateCancelRequested:
			return nil
		}
	}

	if err := s.store.TransitionRunState(ctx, runID, state.RunStateCanceled); err != nil {
		return err
	}
	s.metrics.IncRun("canceled")
	s.logger.Info("run canceled", "event", "run_canceled", "run_id", runID)
	s.reportRun(ctx, runID)
	return nil
}

func isRunTerminal(stateValue state.RunState) bool {
	switch stateValue {
	case state.RunStateSuccess, state.RunStateFailed, state.RunStateCanceled, state.RunStateTimeout, state.RunStateReported:
		return true
	default:
		return false
	}
}

func (s *Service) failRun(ctx context.Context, runID string, logger *slog.Logger, reason string, cause error) error {
	_ = s.store.TransitionRunState(ctx, runID, state.RunStatePlanFailed)
	if logger != nil {
		if cause != nil {
			logger.Error("planner failed", "event", reason, "error", cause)
		} else {
			logger.Error("planner failed", "event", reason)
		}
	}
	s.metrics.IncRun("plan_failed")
	s.metrics.IncFailure("plan_failed")
	if err := s.store.TransitionRunState(ctx, runID, state.RunStateFailed); err != nil && !state.IsTransitionError(err) {
		return err
	}
	s.metrics.IncRun("failed")
	s.reportRun(ctx, runID)
	return nil
}

func (s *Service) reportRun(ctx context.Context, runID string) {
	if s.reporter == nil || runID == "" {
		return
	}
	if err := s.reporter.ReportRun(ctx, runID); err != nil {
		s.metrics.IncFailure("status_report_failed")
		s.logger.Warn("status report failed", "event", "status_report_failed", "run_id", runID, "error", err)
	}
}
