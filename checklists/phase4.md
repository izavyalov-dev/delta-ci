# Phase 4: Scalability and Hardening — Checklist

## 4.1 Worker Concurrency & Graceful Shutdown
- [x] `--max-concurrency` flag (default 4)
- [x] `--shutdown-timeout` flag (default 60s)
- [x] `signal.NotifyContext` for SIGINT/SIGTERM
- [x] Semaphore-based worker pool (`chan struct{}` of size N)
- [x] On shutdown: stop accepting new work, wait for in-flight jobs, exit cleanly

## 4.2 Backpressure & Dead Letters
- [x] `dead_letter_log` table (migration 0017)
- [x] `DeadLetterExpired` sweep function
- [x] `DequeueJobAttemptWithMaxDelivery` — max delivery count filter
- [x] `--max-delivery-count` flag (default 5)
- [x] Adaptive poll interval with exponential backoff

## 4.3 Observability Improvements
- [x] `delta_run_duration_seconds` histogram (state label)
- [x] `delta_job_duration_seconds` histogram (state label)
- [x] `delta_queue_wait_seconds` histogram
- [x] `delta_lease_duration_seconds` histogram (state label)
- [x] `delta_queue_depth` gauge
- [x] `delta_active_leases` gauge
- [x] `delta_dead_letters_total` counter
- [x] `delta_worker_active` gauge
- [x] Periodic gauge collector goroutine (every 15s)
- [x] Metrics methods: Observe*, Set*, Inc*
- [x] `NewServiceWithMetrics` constructor

## 4.4 Profiling & DB Pool Configuration
- [x] `--pprof-enabled` flag
- [x] `--pprof-listen` flag (default :6060)
- [x] pprof endpoints on separate port
- [x] `--db-max-open-conns`, `--db-max-idle-conns`, `--db-conn-max-lifetime` flags
- [x] `openDBWithConfig` for configurable pool settings
- [x] Flags added to both `serve` and `worker` subcommands

## 4.5 Stress & Chaos Testing
- [x] `cmd/loadtest/main.go` — load test binary
- [x] `internal/loadtest/generator.go` — run/job generation
- [x] `internal/loadtest/chaos.go` — fault injection helpers
- [x] `internal/loadtest/loadtest_test.go` — stress test suite
- [x] `TestSustainedLoad` — 100 runs, 4 workers
- [x] `TestBackpressure` — flood queue, verify dead-lettering
- [x] `TestGracefulShutdown` — cancel during active work

## Verification
- [x] `go build ./cmd/...` — all binaries compile
- [x] `go vet ./...` — no issues
- [x] `go fmt ./...` — formatted
- [ ] Start worker with `--max-concurrency=4`, verify parallel execution
- [ ] Send SIGTERM, verify clean shutdown
- [ ] `curl localhost:8080/metrics | grep delta_` — new metrics present
- [ ] `curl localhost:6060/debug/pprof/goroutine` — pprof accessible
- [ ] `go run ./cmd/loadtest` — completes without errors
