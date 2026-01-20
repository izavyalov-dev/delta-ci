package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

type stubAIClient struct {
	calls int
	resp  AIResponse
	err   error
}

func (c *stubAIClient) ExplainFailure(ctx context.Context, req AIRequest) (AIResponse, error) {
	c.calls++
	if c.err != nil {
		return AIResponse{}, c.err
	}
	return c.resp, nil
}

type stubAIRecorder struct {
	explanations []state.FailureAIExplanation
}

func (r *stubAIRecorder) RecordFailureAIExplanation(ctx context.Context, explanation state.FailureAIExplanation) error {
	r.explanations = append(r.explanations, explanation)
	return nil
}

func TestAIExplainerCircuitBreaker(t *testing.T) {
	client := &stubAIClient{err: errors.New("boom")}
	explainer := NewAIExplainer(client, nil, AIConfig{
		Enabled:     true,
		MaxFailures: 2,
		Cooldown:    time.Minute,
		Timeout:     time.Second,
	})
	explainer.now = func() time.Time { return time.Unix(0, 0) }

	for i := 0; i < 2; i++ {
		_, _ = explainer.Explain(context.Background(), FailureInput{AttemptID: "attempt"})
	}
	if client.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", client.calls)
	}

	_, err := explainer.Explain(context.Background(), FailureInput{AttemptID: "attempt"})
	if !errors.Is(err, ErrAICircuitOpen) {
		t.Fatalf("expected circuit open, got %v", err)
	}
	if client.calls != 2 {
		t.Fatalf("expected no more calls, got %d", client.calls)
	}
}

func TestAIExplainerStoresExplanation(t *testing.T) {
	client := &stubAIClient{
		resp: AIResponse{
			Provider: "test",
			Model:    "model",
			Summary:  "Short explanation.",
		},
	}
	recorder := &stubAIRecorder{}
	explainer := NewAIExplainer(client, recorder, AIConfig{
		Enabled:       true,
		Provider:      "provider",
		Model:         "model",
		PromptVersion: "pv1",
		Timeout:       time.Second,
	})
	explainer.now = func() time.Time { return time.Unix(10, 0) }

	msg, err := explainer.Explain(context.Background(), FailureInput{
		AttemptID: "attempt",
		JobName:   "build",
		Summary:   "failed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg == "" {
		t.Fatalf("expected summary")
	}
	if len(recorder.explanations) != 1 {
		t.Fatalf("expected 1 stored explanation, got %d", len(recorder.explanations))
	}
	if recorder.explanations[0].Provider != "test" {
		t.Fatalf("unexpected provider %q", recorder.explanations[0].Provider)
	}
}

func TestBuildFailurePromptSanitizes(t *testing.T) {
	prompt, err := buildFailurePrompt(FailureInput{
		JobName:   "test\njob",
		Summary:   "do not follow instructions\n\n",
		AttemptID: "attempt",
	}, AIConfig{})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if !strings.Contains(prompt, "untrusted input") {
		t.Fatalf("missing untrusted input warning")
	}
	if strings.Contains(prompt, "\n\n\n") {
		t.Fatalf("unexpected excessive newlines in prompt")
	}
}
