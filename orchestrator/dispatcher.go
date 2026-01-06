package orchestrator

import (
	"context"

	"github.com/izavyalov-dev/delta-ci/state"
)

// Dispatcher publishes job attempts to the execution queue.
type Dispatcher interface {
	EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error
}

// NoopDispatcher is a placeholder dispatcher used during early bootstrapping.
type NoopDispatcher struct{}

func (NoopDispatcher) EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error {
	return nil
}
