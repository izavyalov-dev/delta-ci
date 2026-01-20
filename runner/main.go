package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	cacheDir := flag.String("cache-dir", defaultCacheDir(), "Cache directory for runner caches")
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
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

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

	runWorkdir, err := filepath.Abs(*workdir)
	if err != nil {
		logger.Error("resolve workdir", "event", "runner_error", "error", err)
		os.Exit(1)
	}

	cacheUsages, cacheEvents := restoreCaches(*cacheDir, runWorkdir, lease.JobSpec.Caches, logger)

	cmd := exec.CommandContext(runCtx, "sh", "-c", firstStep(lease.JobSpec.Steps))
	cmd.Dir = runWorkdir
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	heartbeatInterval := time.Duration(lease.HeartbeatIntervalSeconds) * time.Second
	if heartbeatInterval <= 0 {
		heartbeatInterval = 20 * time.Second
	}

	var cancelOnce sync.Once
	cancelSignal := make(chan struct{})
	signalCancel := func() {
		cancelOnce.Do(func() {
			close(cancelSignal)
			cancelRun()
		})
	}

	sendHeartbeat := func(ts time.Time) error {
		ack, err := client.Heartbeat(ctx, protocol.Heartbeat{
			Type:     "Heartbeat",
			LeaseID:  lease.LeaseID,
			RunnerID: *runnerID,
			TS:       ts,
		})
		if err != nil {
			return err
		}
		if ack.CancelRequested {
			signalCancel()
		}
		return nil
	}

	start := time.Now().UTC()
	if err := sendHeartbeat(start); err != nil {
		logger.Error("first heartbeat", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	logger.Debug("heartbeat sent", "event", "lease_heartbeat")

	hbDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-hbDone:
				return
			case <-ticker.C:
				if err := sendHeartbeat(time.Now().UTC()); err != nil {
					logger.Error("heartbeat failed", "event", "runner_error", "error", err)
					signalCancel()
					return
				}
			}
		}
	}()

	runnerErr := cmd.Run()
	close(hbDone)
	finished := time.Now().UTC()

	canceled := false
	select {
	case <-cancelSignal:
		canceled = true
	default:
	}

	status := protocol.CompleteStatusSucceeded
	exit := 0
	summary := "succeeded"
	if runnerErr != nil && !canceled {
		status = protocol.CompleteStatusFailed
		exit = exitCode(runnerErr)
		summary = runnerErr.Error()
	}
	if status == protocol.CompleteStatusSucceeded {
		saveCaches(cacheUsages, logger)
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

	if canceled {
		if summary == "succeeded" {
			summary = "canceled"
		}
		cancelAck := protocol.CancelAck{
			Type:        "CancelAck",
			LeaseID:     lease.LeaseID,
			RunnerID:    *runnerID,
			FinalStatus: protocol.CancelFinalStatusCanceled,
			TS:          finished,
			Summary:     summary,
			Artifacts:   artifactsList,
		}
		if err := client.CancelAck(ctx, cancelAck); err != nil {
			logger.Error("cancel ack", "event", "runner_error", "error", err)
			os.Exit(1)
		}
		logger.Info("job canceled", "event", "job_canceled")
		return
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
		Caches:     cacheEvents,
	}
	if err := client.Complete(ctx, complete); err != nil {
		logger.Error("complete", "event", "runner_error", "error", err)
		os.Exit(1)
	}
	logger.Info("job completed", "event", "job_completed", "status", status, "exit_code", exit)
}

type cacheUsage struct {
	spec     protocol.CacheSpec
	root     string
	paths    []cachePathMapping
	hit      bool
	restored bool
}

type cachePathMapping struct {
	cachePath  string
	targetPath string
}

func defaultCacheDir() string {
	if value := os.Getenv("DELTA_CI_CACHE_DIR"); value != "" {
		return value
	}
	return filepath.Join(os.TempDir(), "delta-ci", "cache")
}

func restoreCaches(baseDir, workdir string, specs []protocol.CacheSpec, logger *slog.Logger) ([]cacheUsage, []protocol.CacheEvent) {
	if len(specs) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		logger.Warn("cache dir unavailable", "event", "cache_warning", "error", err)
		return nil, nil
	}

	usages := make([]cacheUsage, 0, len(specs))
	events := make([]protocol.CacheEvent, 0, len(specs))
	for _, spec := range specs {
		if spec.Key == "" || len(spec.Paths) == 0 {
			continue
		}
		root := filepath.Join(baseDir, sanitizeCacheType(spec.Type), hashValue(spec.Type+":"+spec.Key))
		if err := os.MkdirAll(root, 0o755); err != nil {
			logger.Warn("cache root unavailable", "event", "cache_warning", "type", spec.Type, "error", err)
			continue
		}

		usage := cacheUsage{
			spec:  spec,
			root:  root,
			paths: make([]cachePathMapping, 0, len(spec.Paths)),
		}
		for _, path := range spec.Paths {
			target, err := resolveCachePath(path, workdir)
			if err != nil {
				logger.Warn("cache path resolve failed", "event", "cache_warning", "path", path, "error", err)
				continue
			}
			cachePath := filepath.Join(root, hashValue(path))
			usage.paths = append(usage.paths, cachePathMapping{
				cachePath:  cachePath,
				targetPath: target,
			})
			if pathExists(cachePath) {
				usage.hit = true
				if err := restoreCachePath(cachePath, target); err != nil {
					logger.Warn("cache restore failed", "event", "cache_warning", "path", path, "error", err)
				} else {
					usage.restored = true
				}
			}
		}

		events = append(events, protocol.CacheEvent{
			Type:     spec.Type,
			Key:      spec.Key,
			Hit:      usage.hit,
			ReadOnly: spec.ReadOnly,
		})
		usages = append(usages, usage)
	}
	return usages, events
}

func saveCaches(usages []cacheUsage, logger *slog.Logger) {
	for _, usage := range usages {
		if usage.spec.ReadOnly {
			continue
		}
		for _, mapping := range usage.paths {
			if !pathExists(mapping.targetPath) {
				continue
			}
			if err := os.RemoveAll(mapping.cachePath); err != nil {
				logger.Warn("cache clear failed", "event", "cache_warning", "path", mapping.cachePath, "error", err)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(mapping.cachePath), 0o755); err != nil {
				logger.Warn("cache dir create failed", "event", "cache_warning", "path", mapping.cachePath, "error", err)
				continue
			}
			if err := copyPath(mapping.targetPath, mapping.cachePath); err != nil {
				logger.Warn("cache save failed", "event", "cache_warning", "path", mapping.targetPath, "error", err)
			}
		}
	}
}

func resolveCachePath(path, workdir string) (string, error) {
	if path == "" {
		return "", errors.New("empty path")
	}
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workdir, path)
	}
	return filepath.Clean(path), nil
}

func restoreCachePath(cachePath, targetPath string) error {
	info, err := os.Stat(cachePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(cachePath, targetPath)
	}
	return copyFile(cachePath, targetPath, info.Mode())
}

func copyPath(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dest)
	}
	return copyFile(src, dest, info.Mode())
}

func copyDir(src, dest string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Chmod(mode)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sanitizeCacheType(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "cache"
	}
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, "\\", "_")
	return value
}

func hashValue(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
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
