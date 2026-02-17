package notify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/izavyalov-dev/delta-ci/state"
)

// stubStore implements the subset of state.Store used by webhook reporter.
// We test via the real WebhookReporter but with an HTTP test server.

func TestWebhookReporter_SendsPayload(t *testing.T) {
	var received WebhookPayload
	var headers http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers = r.Header
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()

	// Create a run that reaches a terminal state.
	runID := createTerminalRun(t, ctx, store)

	reporter := NewWebhookReporter(store, WebhookConfig{
		URL:    srv.URL,
		Events: EventFilterTerminalOnly,
	}, testLogger(), "https://ci.example.com")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}

	if received.RunID != runID {
		t.Errorf("run_id = %q, want %q", received.RunID, runID)
	}
	if received.State != string(state.RunStateSuccess) {
		t.Errorf("state = %q, want SUCCESS", received.State)
	}
	if received.DashboardURL != "https://ci.example.com/runs/"+runID {
		t.Errorf("dashboard_url = %q", received.DashboardURL)
	}
	if headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", headers.Get("Content-Type"))
	}
}

func TestWebhookReporter_HMACSigning(t *testing.T) {
	secret := "test-secret"
	var receivedSig string
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-DeltaCI-Signature")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()
	runID := createTerminalRun(t, ctx, store)

	reporter := NewWebhookReporter(store, WebhookConfig{
		URL:    srv.URL,
		Secret: secret,
		Events: EventFilterTerminalOnly,
	}, testLogger(), "")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}

	// Verify HMAC.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if receivedSig != expected {
		t.Errorf("signature = %q, want %q", receivedSig, expected)
	}
}

func TestWebhookReporter_SkipsNonTerminal(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()
	runID := createNonTerminalRun(t, ctx, store)

	reporter := NewWebhookReporter(store, WebhookConfig{
		URL:    srv.URL,
		Events: EventFilterTerminalOnly,
	}, testLogger(), "")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}
	if called {
		t.Error("webhook should not fire for non-terminal state")
	}
}

func TestWebhookReporter_AllEvents(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := newTestStore(t)
	ctx := context.Background()
	runID := createNonTerminalRun(t, ctx, store)

	reporter := NewWebhookReporter(store, WebhookConfig{
		URL:    srv.URL,
		Events: EventFilterAll,
	}, testLogger(), "")

	if err := reporter.ReportRun(ctx, runID); err != nil {
		t.Fatalf("ReportRun: %v", err)
	}
	if !called {
		t.Error("webhook should fire for non-terminal state with all events filter")
	}
}
