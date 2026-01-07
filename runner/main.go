package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/protocol"
	"github.com/izavyalov-dev/delta-ci/runner/artifacts"
	"github.com/izavyalov-dev/delta-ci/runner/transport"
)

func main() {
	orchestrator := flag.String("orchestrator", "http://localhost:8080", "Orchestrator base URL")
	runnerID := flag.String("runner-id", "", "Runner identity (required)")
	leasePath := flag.String("lease", "", "Path to LeaseGranted JSON payload")
	workdir := flag.String("workdir", ".", "Working directory for job execution")
	logPath := flag.String("log", "runner.log", "Path to write combined stdout/stderr log")
	s3Bucket := flag.String("s3-bucket", "", "S3 bucket for log uploads")
	s3Prefix := flag.String("s3-prefix", "", "S3 key prefix for log uploads")
	s3Region := flag.String("s3-region", "", "AWS region for S3 (optional)")
	flag.Parse()

	baseLogger := observability.NewLogger("runner")

	if *runnerID == "" {
		baseLogger.Error("runner-id is required", "event", "runner_error")
		os.Exit(1)
	}
	if *leasePath == "" {
		baseLogger.Error("lease path is required", "event", "runner_error")
		os.Exit(1)
	}

	leaseFile, err := os.ReadFile(*leasePath)
	if err != nil {
		baseLogger.Error("read lease file", "event", "runner_error", "error", err)
		os.Exit(1)
	}

	var lease protocol.LeaseGranted
	if err := json.Unmarshal(leaseFile, &lease); err != nil {
		baseLogger.Error("parse lease", "event", "runner_error", "error", err)
		os.Exit(1)
	}

	logger := baseLogger
	logger = observability.WithRun(logger, lease.RunID)
	logger = observability.WithJob(logger, lease.JobID)
	logger = observability.WithLease(logger, lease.LeaseID)

	logWriter, err := os.Create(filepath.Clean(*logPath))
	if err != nil {
		logger.Error("open log file", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	defer logWriter.Close()

	client := transport.NewHTTPClient(*orchestrator)
	ctx := context.Background()

	ack := protocol.AckLease{
		Type:       "AckLease",
		JobID:      lease.JobID,
		LeaseID:    lease.LeaseID,
		RunnerID:   *runnerID,
		AcceptedAt: time.Now().UTC(),
	}
	if err := client.AckLease(ctx, ack); err != nil {
		logger.Error("ack lease", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	logger.Info("lease acknowledged", "event", "lease_acknowledged")

	cmd := exec.CommandContext(ctx, "sh", "-c", firstStep(lease.JobSpec.Steps))
	cmd.Dir = *workdir
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	start := time.Now().UTC()
	if err := client.Heartbeat(ctx, protocol.Heartbeat{
		Type:     "Heartbeat",
		LeaseID:  lease.LeaseID,
		RunnerID: *runnerID,
		TS:       start,
	}); err != nil {
		logger.Error("first heartbeat", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	logger.Debug("heartbeat sent", "event", "lease_heartbeat")

	runnerErr := cmd.Run()
	finished := time.Now().UTC()

	status := protocol.CompleteStatusSucceeded
	exit := 0
	summary := "succeeded"
	if runnerErr != nil {
		status = protocol.CompleteStatusFailed
		exit = exitCode(runnerErr)
		summary = runnerErr.Error()
	}

	if err := logWriter.Sync(); err != nil {
		logger.Warn("sync log file", "event", "runner_warning", "error", err)
	}

	var artifactsList []protocol.ArtifactRef
	if *s3Bucket != "" {
		uploader, err := artifacts.NewS3Uploader(ctx, artifacts.S3Config{
			Bucket: *s3Bucket,
			Prefix: *s3Prefix,
			Region: *s3Region,
		})
		if err != nil {
			logger.Warn("init s3 uploader", "event", "artifact_upload_failed", "error", err)
		} else {
			uri, err := uploader.UploadLog(ctx, lease.RunID, lease.JobID, *logPath)
			if err != nil {
				logger.Warn("upload log", "event", "artifact_upload_failed", "error", err)
			} else {
				artifactsList = append(artifactsList, protocol.ArtifactRef{
					Type: "log",
					URI:  uri,
				})
				logger.Info("log uploaded", "event", "artifact_uploaded", "uri", uri)
			}
		}
	}

	complete := protocol.Complete{
		Type:       "Complete",
		LeaseID:    lease.LeaseID,
		RunnerID:   *runnerID,
		Status:     status,
		ExitCode:   exit,
		FinishedAt: finished,
		Summary:    summary,
		Artifacts:  artifactsList,
	}
	if err := client.Complete(ctx, complete); err != nil {
		logger.Error("complete", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	logger.Info("job completed", "event", "job_completed", "status", status, "exit_code", exit)
}

func firstStep(steps []string) string {
	if len(steps) == 0 {
		return "echo \"no steps provided\""
	}
	return steps[0]
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return 1
}
