package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/izavyalov-dev/delta-ci/state"
)

func TestSlackReporter_SendsBlockKit(t *testing.T) {
	var received slackMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()
	runID := createTerminalRun(t, ctx, store)

	reporter := NewSlackReporter(store, SlackConfig{
		WebhookURL: srv.URL,
		Events:     EventFilterTerminalOnly,
	}, testLogger(), "https://ci.example.com")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}

	if len(received.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(received.Blocks))
	}

	// First block should contain SUCCESS.
	if received.Blocks[0].Text == nil {
		t.Fatal("first block has no text")
	}
	text := received.Blocks[0].Text.Text
	if !containsSubstring(text, string(state.RunStateSuccess)) {
		t.Errorf("header block = %q, want SUCCESS", text)
	}
}

func TestSlackReporter_SkipsNonTerminal(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()
	runID := createNonTerminalRun(t, ctx, store)

	reporter := NewSlackReporter(store, SlackConfig{
		WebhookURL: srv.URL,
		Events:     EventFilterTerminalOnly,
	}, testLogger(), "")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}
	if called {
		t.Error("slack should not fire for non-terminal state")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
