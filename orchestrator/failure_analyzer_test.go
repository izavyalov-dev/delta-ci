package orchestrator

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

func TestRuleBasedFailureAnalyzerSignals(t *testing.T) {
	analyzer := NewRuleBasedFailureAnalyzer()
	start := time.Now().Add(-3 * time.Second)
	finish := start.Add(3 * time.Second)

	explanation, err := analyzer.Analyze(context.Background(), FailureInput{
		RunID:         "run",
		JobID:         "job",
		JobName:       "test",
		AttemptID:     "attempt",
		AttemptNumber: 2,
		Status:        protocol.CompleteStatusFailed,
		ExitCode:      1,
		Summary:       "exit status 1",
		Artifacts: []state.ArtifactRef{
			{Type: "log", URI: "s3://logs/log.txt"},
			{Type: "junit", URI: "s3://logs/junit.xml"},
		},
		CacheEvents: []protocol.CacheEvent{
			{Type: "deps", Key: "k1", Hit: false, ReadOnly: true},
			{Type: "deps", Key: "k2", Hit: true},
		},
		StartedAt:  &start,
		FinishedAt: &finish,
	})
	if err != nil {
		t.Fatalf("analyze failure: %v", err)
	}
	if explanation == nil {
		t.Fatal("expected explanation")
	}
	if explanation.RuleVersion != failureRuleVersion {
		t.Fatalf("expected rule version %q, got %q", failureRuleVersion, explanation.RuleVersion)
	}
	if explanation.Signals.AttemptNumber != 2 {
		t.Fatalf("expected attempt number 2, got %d", explanation.Signals.AttemptNumber)
	}
	if explanation.Signals.DurationSeconds != 3 {
		t.Fatalf("expected duration 3s, got %d", explanation.Signals.DurationSeconds)
	}
	if !explanation.Signals.HasLog {
		t.Fatalf("expected log signal")
	}
	expectedArtifacts := []string{"junit", "log"}
	if !reflect.DeepEqual(explanation.Signals.ArtifactTypes, expectedArtifacts) {
		t.Fatalf("unexpected artifact types: %v", explanation.Signals.ArtifactTypes)
	}
	if len(explanation.Signals.CacheEvents) != 2 {
		t.Fatalf("expected 2 cache events, got %d", len(explanation.Signals.CacheEvents))
	}
	if explanation.Signals.CacheEvents[0].Key != "k1" {
		t.Fatalf("unexpected cache event order: %v", explanation.Signals.CacheEvents)
	}
	if !strings.Contains(explanation.Details, "Attempt: 2") {
		t.Fatalf("missing attempt detail: %s", explanation.Details)
	}
	if !strings.Contains(explanation.Details, "Duration: 3s") {
		t.Fatalf("missing duration detail: %s", explanation.Details)
	}
}

func TestClassifyFailureExitCode137(t *testing.T) {
	category, confidence, summary := classifyFailure("build", "process killed", 137)
	if category != state.FailureCategoryInfra {
		t.Fatalf("expected infra category, got %s", category)
	}
	if confidence != state.FailureConfidenceHigh {
		t.Fatalf("expected high confidence, got %s", confidence)
	}
	if !strings.Contains(summary, "Resource exhaustion") {
		t.Fatalf("unexpected summary: %s", summary)
	}
}
