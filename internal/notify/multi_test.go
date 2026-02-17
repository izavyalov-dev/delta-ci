package notify

import (
	"context"
	"errors"
	"testing"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
)

type fakeReporter struct {
	calls []string
	err   error
}

func (f *fakeReporter) ReportRun(_ context.Context, runID string) error {
	f.calls = append(f.calls, runID)
	return f.err
}

func TestMultiReporter_FansOut(t *testing.T) {
	a := &fakeReporter{}
	b := &fakeReporter{}
	m := NewMultiReporter(a, b)

	if err := m.ReportRun(context.Background(), "run-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(a.calls) != 1 || a.calls[0] != "run-1" {
		t.Errorf("reporter a: got %v", a.calls)
	}
	if len(b.calls) != 1 || b.calls[0] != "run-1" {
		t.Errorf("reporter b: got %v", b.calls)
	}
}

func TestMultiReporter_CollectsErrors(t *testing.T) {
	a := &fakeReporter{err: errors.New("fail-a")}
	b := &fakeReporter{err: errors.New("fail-b")}
	m := NewMultiReporter(a, b)

	err := m.ReportRun(context.Background(), "run-2")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, a.err) || !errors.Is(err, b.err) {
		t.Errorf("expected joined errors, got: %v", err)
	}
}

func TestMultiReporter_FiltersNilAndNoop(t *testing.T) {
	a := &fakeReporter{}
	m := NewMultiReporter(nil, a, orchestrator.NoopStatusReporter{}, nil)

	if m.Len() != 1 {
		t.Errorf("expected 1 reporter, got %d", m.Len())
	}
}

func TestMultiReporter_Empty(t *testing.T) {
	m := NewMultiReporter()
	if err := m.ReportRun(context.Background(), "run-3"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
