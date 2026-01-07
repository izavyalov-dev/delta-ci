package orchestrator

import (
	"context"
	"errors"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

// Dispatcher publishes job attempts to the execution queue.
type Dispatcher interface {
	EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error
}

// QueueDispatcher publishes attempts to the Postgres-backed queue.
type QueueDispatcher struct {
	store *state.Store
}

func NewQueueDispatcher(store *state.Store) QueueDispatcher {
	return QueueDispatcher{store: store}
}

func (d QueueDispatcher) EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error {
	if d.store == nil {
		return errors.New("queue dispatcher requires store")
	}
	return d.store.EnqueueJobAttempt(ctx, attempt.ID, time.Now().UTC())
}

// NoopDispatcher is a placeholder dispatcher used during early bootstrapping.
type NoopDispatcher struct{}

func (NoopDispatcher) EnqueueJobAttempt(ctx context.Context, attempt state.JobAttempt) error {
	return nil
}
