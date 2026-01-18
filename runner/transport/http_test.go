package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

func TestHTTPClientPostsJSON(t *testing.T) {
	var received []byte
	srv := mustTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var err error
		received, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
	}))
	if srv == nil {
		return
	}
	defer srv.Close()

	client := NewHTTPClient(srv.URL)
	err := client.AckLease(context.Background(), protocol.AckLease{
		Type:       "AckLease",
		JobID:      "job",
		LeaseID:    "lease",
		RunnerID:   "runner",
		AcceptedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("post: %v", err)
	}

	if len(received) == 0 {
		t.Fatal("expected payload to be sent")
	}
}

// mustTestServer starts a test server or skips if the sandbox disallows listening.
func mustTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("test server unavailable in sandbox: %v", r)
		}
	}()
	return httptest.NewServer(handler)
}
