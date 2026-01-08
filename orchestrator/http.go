package orchestrator

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/protocol"
)

// NewHTTPHandler wires minimal internal endpoints for runner protocol and metrics.
func NewHTTPHandler(service *Service, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = observability.NewLogger("orchestrator.http")
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

	return mux
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
