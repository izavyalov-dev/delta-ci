package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/internal/vcs/github"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/state"
)

// HTTPConfig controls public webhook handling.
type HTTPConfig struct {
	GitHubWebhookSecret   string
	GitHubWebhookMaxBytes int64
}

// NewHTTPHandler wires minimal internal endpoints for runner protocol and metrics.
func NewHTTPHandler(service *Service, logger *slog.Logger, config HTTPConfig) http.Handler {
	if logger == nil {
		logger = observability.NewLogger("orchestrator.http")
	}
	if config.GitHubWebhookMaxBytes <= 0 {
		config.GitHubWebhookMaxBytes = 1 << 20
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", observability.MetricsHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/internal/ack-lease", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var msg protocol.AckLease
		if err := decodeJSON(r, &msg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := service.AckLease(r.Context(), msg); err != nil {
			if errors.Is(err, ErrStaleLease) {
				writeError(w, http.StatusConflict, err)
				return
			}
			logger.Error("ack lease failed", "event", "ack_lease_failed", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/internal/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var msg protocol.Heartbeat
		if err := decodeJSON(r, &msg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ack, err := service.HandleHeartbeat(r.Context(), msg)
		if err != nil {
			if errors.Is(err, ErrStaleLease) {
				writeError(w, http.StatusConflict, err)
				return
			}
			logger.Error("heartbeat failed", "event", "heartbeat_failed", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, ack)
	})

	mux.HandleFunc("/api/v1/internal/complete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var msg protocol.Complete
		if err := decodeJSON(r, &msg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := service.CompleteLease(r.Context(), msg); err != nil {
			if errors.Is(err, ErrStaleLease) {
				writeError(w, http.StatusConflict, err)
				return
			}
			logger.Error("complete failed", "event", "complete_failed", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/internal/cancel-ack", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var msg protocol.CancelAck
		if err := decodeJSON(r, &msg); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := service.CancelLease(r.Context(), msg); err != nil {
			if errors.Is(err, ErrStaleLease) {
				writeError(w, http.StatusConflict, err)
				return
			}
			logger.Error("cancel ack failed", "event", "cancel_ack_failed", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/v1/webhooks/github", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if config.GitHubWebhookSecret == "" {
			writeError(w, http.StatusServiceUnavailable, errors.New("github webhook secret not configured"))
			return
		}
		body, err := readBody(r, config.GitHubWebhookMaxBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		signature := r.Header.Get("X-Hub-Signature-256")
		if signature == "" {
			signature = r.Header.Get("X-Hub-Signature")
		}
		valid, err := github.VerifySignature(config.GitHubWebhookSecret, body, signature)
		if err != nil || !valid {
			logger.Warn("github webhook signature rejected", "event", "webhook_signature_invalid", "error", err)
			writeError(w, http.StatusUnauthorized, errors.New("invalid webhook signature"))
			return
		}

		eventType := r.Header.Get("X-GitHub-Event")
		if eventType == "" {
			writeError(w, http.StatusBadRequest, errors.New("missing github event header"))
			return
		}
		normalized, triggerRun, err := github.NormalizeEvent(eventType, body)
		if err != nil {
			logger.Warn("github webhook normalization failed", "event", "webhook_normalize_failed", "error", err)
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if !triggerRun {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		eventKey, err := github.ComputeEventKey(normalized.RepoID, normalized.CommitSHA, normalized.EventType, normalized.PRNumber)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		runDetails, created, err := service.CreateRunFromTrigger(r.Context(), CreateRunRequest{
			RepoID:    normalized.RepoID,
			Ref:       normalized.Ref,
			CommitSHA: normalized.CommitSHA,
		}, state.RunTrigger{
			Provider:  "github",
			EventKey:  eventKey,
			EventType: normalized.EventType,
			RepoID:    normalized.RepoID,
			RepoOwner: normalized.RepoOwner,
			RepoName:  normalized.RepoName,
			PRNumber:  normalized.PRNumber,
		})
		if err != nil {
			logger.Error("github webhook run creation failed", "event", "webhook_run_failed", "error", err)
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		status := http.StatusAccepted
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, map[string]string{
			"run_id": runDetails.Run.ID,
			"state":  string(runDetails.Run.State),
		})
	})

	mux.HandleFunc("/api/v1/runs/", func(w http.ResponseWriter, r *http.Request) {
		runID, action, ok := parseRunPath(r.URL.Path)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Method == http.MethodGet {
			if action != "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			details, err := service.GetRunDetails(r.Context(), runID)
			if err != nil {
				if errors.Is(err, state.ErrNotFound) {
					writeError(w, http.StatusNotFound, err)
					return
				}
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, details)
			return
		}

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if action == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		switch action {
		case "cancel":
			details, err := service.CancelRun(r.Context(), runID)
			if err != nil {
				if state.IsTransitionError(err) {
					writeError(w, http.StatusConflict, err)
					return
				}
				if errors.Is(err, ErrInvalidRunState) {
					writeError(w, http.StatusConflict, err)
					return
				}
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{
				"run_id": details.Run.ID,
				"state":  string(details.Run.State),
			})
		case "rerun":
			idempotencyKey := r.Header.Get("Idempotency-Key")
			if idempotencyKey == "" {
				writeError(w, http.StatusBadRequest, errors.New("Idempotency-Key header required"))
				return
			}
			details, created, err := service.RerunRun(r.Context(), runID, idempotencyKey)
			if err != nil {
				if state.IsTransitionError(err) {
					writeError(w, http.StatusConflict, err)
					return
				}
				writeError(w, http.StatusBadRequest, err)
				return
			}
			status := http.StatusOK
			if created {
				status = http.StatusCreated
			}
			writeJSON(w, status, map[string]string{
				"run_id":         details.Run.ID,
				"original_run":   runID,
				"state":          string(details.Run.State),
				"created":        fmt.Sprintf("%t", created),
				"idempotencyKey": idempotencyKey,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	return mux
}

func readBody(r *http.Request, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return io.ReadAll(r.Body)
	}
	limit := maxBytes + 1
	body, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, errors.New("payload too large")
	}
	return body, nil
}

func parseRunPath(path string) (string, string, bool) {
	path = strings.TrimPrefix(path, "/api/v1/runs/")
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", false
		}
		return parts[0], "", true
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", false
		}
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}

func decodeJSON(r *http.Request, target any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
