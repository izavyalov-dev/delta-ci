package observability

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
)

// NewLogger returns a JSON logger with a component field attached.
func NewLogger(component string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := slog.New(handler)
	if component != "" {
		logger = logger.With("component", component)
	}
	return logger
}

func WithRun(logger *slog.Logger, runID string) *slog.Logger {
	if logger == nil || runID == "" {
		return logger
	}
	return logger.With("run_id", runID)
}

func WithJob(logger *slog.Logger, jobID string) *slog.Logger {
	if logger == nil || jobID == "" {
		return logger
	}
	return logger.With("job_id", jobID)
}

func WithLease(logger *slog.Logger, leaseID string) *slog.Logger {
	if logger == nil || leaseID == "" {
		return logger
	}
	return logger.With("lease_id_hash", hashLeaseID(leaseID))
}

func hashLeaseID(leaseID string) string {
	sum := sha256.Sum256([]byte(leaseID))
	return hex.EncodeToString(sum[:8])
}
