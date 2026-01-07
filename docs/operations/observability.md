# Observability

This document defines observability expectations for Delta CI:
- logs
- metrics
- traces
- audit events

Observability is required for correctness, security, and operability.

---

## Goals

Observability must enable operators to:
- understand system health
- debug failures quickly
- detect regressions
- investigate security incidents
- measure SLOs

---

## Structured Logging

All control plane components must emit structured logs with:
- `run_id`
- `job_id`
- `lease_id_hash` (hash only; lease IDs are treated as secrets)
- `repo_id`
- `component`
- `severity`
- `event`

Phase 0 emits JSON logs by default.

Logs must never include:
- secrets
- raw environment variables
- tokens or lease IDs in plaintext

---

## Metrics

### Core Metrics (Required)

Control plane:
- runs created / finalized
- run duration
- jobs queued / running / succeeded / failed / canceled
- retries per job
- lease grants / expirations / stale completions
- cancel requests and forced cancels

Data plane:
- runner starts / terminations
- job execution duration
- artifact upload duration and size
- heartbeat success/failure rate

Storage:
- artifact store latency
- DB latency and error rate
- queue latency and depth

### Phase 0 Metrics (Implemented)

- `delta_runs_total{state=...}`
- `delta_jobs_total{state=...}`
- `delta_leases_total{state=...}`
- `delta_failures_total{type=...}`

---

### SLO Candidates

- 95th percentile time from push/PR update to first signal
- 95th percentile run completion time for “common changes”
- runner lease expiration rate < threshold
- control plane availability

SLOs should be defined once dogfooding produces real baselines.

---

## Tracing

Distributed tracing should cover:
- webhook → run creation
- planning → plan persisted
- job enqueue → lease grant
- heartbeat loop
- completion → reporting

Minimum trace correlation fields:
- `run_id`
- `job_id`
- `lease_id`

---

## Audit Logging

Audit events are required for security and forensics.

Audit events should include:
- who initiated a run / cancel / rerun
- secret access events (metadata only)
- policy violations
- lease grant/revoke/expire
- forced cancellations
- status reporting actions

Audit logs must be:
- immutable
- retained per policy
- queryable by run/job/repo

---

## Dashboards (Recommended)

- system overview (runs, failures, latency)
- queue and runner capacity
- lease health (expirations, stale messages)
- artifact store performance
- repo-level breakdown (top slow repos, flaky jobs)

---

## Alerts (Recommended)

High-signal alerts:
- orchestrator cannot persist state (DB down)
- queue backlog exceeding threshold
- lease expirations spike
- artifact store failures spike
- status reporter failure rate > threshold

Avoid noisy alerts based on single run failures.

---

## Summary

Delta CI observability is:
- structured
- correlated by IDs
- focused on leases and state transitions
- security-aware by default
