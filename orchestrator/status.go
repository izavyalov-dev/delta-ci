package orchestrator

import "context"

// StatusReporter publishes run status to external systems.
type StatusReporter interface {
	ReportRun(ctx context.Context, runID string) error
}

// NoopStatusReporter ignores status updates.
type NoopStatusReporter struct{}

func (NoopStatusReporter) ReportRun(ctx context.Context, runID string) error {
	return nil
}
