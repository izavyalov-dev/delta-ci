package loadtest

import (
	"context"
	"fmt"
	"time"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

// SimulateJobSuccess dequeues, grants, acks, and completes a single job successfully.
func SimulateJobSuccess(ctx context.Context, service *orchestrator.Service, store *state.Store, visibilityTimeout time.Duration) error {
	attemptID, err := service.DequeueJobAttempt(ctx, visibilityTimeout)
	if err != nil {
		return fmt.Errorf("dequeue: %w", err)
	}

	lease, err := service.GrantLease(ctx, orchestrator.GrantLeaseRequest{
		AttemptID:        attemptID,
		RunnerID:         "chaos-worker",
		TTLSeconds:       120,
		HeartbeatSeconds: 30,
	})
	if err != nil {
		return fmt.Errorf("grant lease: %w", err)
	}

	if err := service.AckLease(ctx, protocol.AckLease{
		Type:     "AckLease",
		LeaseID:  lease.LeaseID,
		RunnerID: "chaos-worker",
	}); err != nil {
		return fmt.Errorf("ack lease: %w", err)
	}

	if err := service.CompleteLease(ctx, protocol.Complete{
		Type:       "Complete",
		LeaseID:    lease.LeaseID,
		Status:     protocol.CompleteStatusSucceeded,
		ExitCode:   0,
		FinishedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("complete: %w", err)
	}

	return nil
}

// FloodQueue creates many runs to saturate the queue without consuming them.
func FloodQueue(ctx context.Context, service *orchestrator.Service, count int) ([]string, error) {
	var runIDs []string
	for i := 0; i < count; i++ {
		details, err := service.CreateRun(ctx, orchestrator.CreateRunRequest{
			RepoID:    "chaos-flood-repo",
			Ref:       "refs/heads/chaos",
			CommitSHA: fmt.Sprintf("chaos-sha-%d-%d", time.Now().UnixNano(), i),
		})
		if err != nil {
			return nil, fmt.Errorf("flood run %d: %w", i, err)
		}
		runIDs = append(runIDs, details.Run.ID)
	}
	return runIDs, nil
}

// SetDeliveryCount directly sets the delivery_count on a queue item for testing.
func SetDeliveryCount(ctx context.Context, store *state.Store, attemptID string, count int) error {
	return store.SetQueueDeliveryCount(ctx, attemptID, count)
}
