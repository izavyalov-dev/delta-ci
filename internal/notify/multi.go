package notify

import (
	"context"
	"errors"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
)

// MultiReporter fans out ReportRun calls to multiple reporters.
// All reporters are called regardless of individual errors. Errors are joined.
type MultiReporter struct {
	reporters []orchestrator.StatusReporter
}

// NewMultiReporter creates a MultiReporter from the given reporters.
// Nil and NoopStatusReporter entries are filtered out.
func NewMultiReporter(reporters ...orchestrator.StatusReporter) *MultiReporter {
	var filtered []orchestrator.StatusReporter
	for _, r := range reporters {
		if r == nil {
			continue
		}
		if _, ok := r.(orchestrator.NoopStatusReporter); ok {
			continue
		}
		filtered = append(filtered, r)
	}
	return &MultiReporter{reporters: filtered}
}

func (m *MultiReporter) ReportRun(ctx context.Context, runID string) error {
	var errs []error
	for _, r := range m.reporters {
		if err := r.ReportRun(ctx, runID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Len returns the number of active reporters.
func (m *MultiReporter) Len() int {
	return len(m.reporters)
}
