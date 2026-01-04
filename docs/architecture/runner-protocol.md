# Runner Protocol

This document specifies the **Runner ↔ Orchestrator** protocol used by Delta CI.

The protocol is designed to be:
- reliable under runner crashes and network failures
- safe against duplicate execution and stale completions
- simple enough to implement in multiple languages
- compatible with queue-based dispatch

This document defines:
- message semantics
- required fields
- lifecycle rules
- cancellation behavior
- failure modes and expected handling

> If a behavior is not documented here, it must be treated as **undefined**.

---

## Scope

This protocol covers the interaction between:
- **Runner** (ephemeral executor)
- **Orchestrator** (state authority)

It includes:
- leasing jobs (exclusive execution rights)
- heartbeats (liveness and progress)
- completion (success/failure)
- cancellation (graceful stop + finalization)

It does not define:
- job specification format (defined elsewhere)
- artifact storage APIs (runner uploads to a store, referenced here)
- VCS integration (status reporting is external to this protocol)

---

## Protocol Overview

### Key Ideas

1. **Lease-based execution**
   - A job may only be executed by a runner holding an active lease.
   - The lease has a TTL and must be refreshed by heartbeats.

2. **Fencing**
   - The `lease_id` acts as a **fencing token**.
   - Orchestrator accepts `Complete` only from the **current active lease**.

3. **At-least-once dispatch**
   - Job dispatch mechanisms may deliver the same job more than once.
   - The lease model prevents double-finalization.

4. **Two-phase cancellation**
   - Cancel is requested by Orchestrator.
   - Runner acknowledges cancellation and reports final state.

---

## Entities and Identifiers

### Runner
A runner instance that can execute one job at a time.

Required identity fields:
- `runner_id` (stable within runner lifetime, unique)
- `capabilities` (tags describing environment: os/arch/toolchain)

### Job
A unit of work created by the planner.

Key identifiers:
- `job_id` (globally unique)
- `run_id` (the run that owns the job)

### Lease
The exclusive right to execute a job.

Key identifiers:
- `lease_id` (globally unique, unguessable)
- `lease_ttl_seconds` (time before lease expires without heartbeat)

---

## Transport Models

Delta CI supports two transport patterns:

### A) Queue-based dispatch + Orchestrator control (recommended)
- Runner leases jobs via a queue (pull model).
- Heartbeat/Complete go to Orchestrator.
- Cancel can be delivered via a control queue/topic and/or via heartbeat response.

### B) Orchestrator-only API
- Runner requests leases directly from Orchestrator.
- Cancel is observed via heartbeat responses unless a push channel exists.

This document defines message semantics independent of transport.
Implementations must ensure the same state and ordering guarantees.

---

## State and Ownership Rules

### Orchestrator is authoritative for:
- lease validity
- job final status
- retries and attempts
- cancel finalization

### Runner is authoritative for:
- actual execution and exit code
- produced artifacts (as references)
- best-effort progress reporting

### Hard Rules
- Runner MUST NOT execute a job without a granted lease.
- Runner MUST NOT send `Complete` after lease expiration (if it does, Orchestrator must reject it).
- Orchestrator MUST treat any message with an unknown or expired `lease_id` as stale.

---

## Message Types

The protocol defines the following actions:

1. `Lease` (runner obtains an exclusive lease)
2. `AckLease` (runner confirms it accepted the lease)
3. `Heartbeat` (runner keeps lease alive and reports progress)
4. `Complete` (runner finalizes execution outcome)
5. `CancelRequested` (orchestrator requests stop)
6. `CancelAck` (runner confirms stop and final state)

---

## Message Schemas (JSON)

Schemas are shown in JSON for readability.
Implementations may use JSON, protobuf, or another encoding, but fields and semantics must match.

### 1) LeaseGranted

Sent to the runner when a lease is granted.

```json
{
  "type": "LeaseGranted",
  "job_id": "job_123",
  "run_id": "run_456",
  "lease_id": "lease_abc",
  "lease_ttl_seconds": 120,
  "heartbeat_interval_seconds": 20,
  "max_runtime_seconds": 3600,
  "job_spec": {
    "name": "unit-tests",
    "workdir": ".",
    "image": "ghcr.io/delta-ci/runner-dotnet:9",
    "steps": ["dotnet test -c Release"],
    "env": { "CI": "true" },
    "artifacts": [
      { "type": "junit", "path_glob": "**/TestResults/*.trx" }
    ],
    "caches": [
      { "type": "deps", "key": "nuget:{lock_hash}", "paths": ["~/.nuget/packages"] }
    ]
  },
  "cancel_token": "cancel_tok_optional"
}
```

**Notes**
*	lease_id must be unguessable (UUIDv4 is ok, crypto-random preferred).
*	job_spec may be embedded or referenced via job_spec_ref (URI) for large specs.

### 2) AckLease

Runner confirms it accepted the lease and is starting work.

```json
{
  "type": "AckLease",
  "job_id": "job_123",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "accepted_at": "2026-01-04T08:00:00Z"
}
```

**Semantics**
*	Orchestrator moves the job into an “active/leased” state on receipt.
*	If AckLease is not received within a short window (e.g., 30s), Orchestrator may revoke the lease.

### 3) Heartbeat

Runner reports liveness and progress. Extends lease TTL.

```json
{
  "type": "Heartbeat",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "progress": {
    "percent": 35,
    "current_step": "dotnet test -c Release",
    "step_index": 0,
    "message": "Running tests..."
  },
  "log_cursor": {
    "bytes_sent": 1048576
  },
  "ts": "2026-01-04T08:00:20Z"
}
```

Heartbeat Response

```json
{
  "type": "HeartbeatAck",
  "lease_id": "lease_abc",
  "extend_lease": true,
  "new_lease_ttl_seconds": 120,
  "cancel_requested": false,
  "cancel_deadline_seconds": 0
}
```

**Semantics**
*	Orchestrator should renew the lease on each heartbeat.
*	If the lease is expired, Orchestrator replies with extend_lease=false and stale=true (see “Stale Lease Handling”).

### 4) Complete

Runner reports final result and ends the lease.

```json
{
  "type": "Complete",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "status": "SUCCEEDED",
  "exit_code": 0,
  "timings": {
    "started_at": "2026-01-04T08:00:05Z",
    "finished_at": "2026-01-04T08:03:12Z"
  },
  "artifacts": [
    { "type": "log", "uri": "s3://delta-ci-artifacts/runs/run_456/jobs/job_123/log.txt" },
    { "type": "junit", "uri": "s3://delta-ci-artifacts/runs/run_456/jobs/job_123/test.trx" }
  ],
  "summary": "All tests passed."
}
```

Complete Response

```json
{
  "type": "CompleteAck",
  "lease_id": "lease_abc",
  "accepted": true
}
```

**Semantics**
*	Orchestrator finalizes the job status upon accepting Complete.
*	After CompleteAck, runner should stop heartbeating and may terminate.

### 5) CancelRequested

Orchestrator requests job cancellation.

This can be delivered via:
*	control queue/topic
*	or HeartbeatAck.cancel_requested=true

```json
{
  "type": "CancelRequested",
  "lease_id": "lease_abc",
  "job_id": "job_123",
  "reason": "RUN_CANCELED",
  "deadline_seconds": 30,
  "ts": "2026-01-04T08:01:10Z"
}
```

**Semantics**
*	Runner must attempt a graceful stop within deadline_seconds.
*	After deadline, Orchestrator may finalize the job as canceled even without acknowledgment.

### 6) CancelAck

Runner confirms it stopped execution and returns final cancellation info.

```json
{
  "type": "CancelAck",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "final_status": "CANCELED",
  "ts": "2026-01-04T08:01:25Z",
  "artifacts": [
    { "type": "log", "uri": "s3://delta-ci-artifacts/runs/run_456/jobs/job_123/log.partial.txt" }
  ],
  "summary": "Canceled during step: dotnet test."
}
```

**Semantics**
*	Orchestrator finalizes job as CANCELED if lease is still valid and cancel was requested.
*	If the job already completed, Orchestrator may reject the cancellation as stale.

## Stale Lease Handling

A lease is stale if:
*	TTL expired without heartbeat, or
*	Orchestrator revoked the lease, or
*	a new lease was granted for the same job attempt

When a runner sends Heartbeat or Complete with a stale lease:
*	Orchestrator MUST NOT change job final state
*	Orchestrator should reply with an explicit stale signal:

Example response:
```json
{
  "type": "StaleLease",
  "lease_id": "lease_abc",
  "reason": "LEASE_EXPIRED"
}
```

Runner behavior on StaleLease:
*	stop execution if still running (best effort)
*	stop heartbeating
*	release local resources
*	do not attempt Complete again

## Recommended Defaults

These defaults are intended for most deployments:
*	lease_ttl_seconds: 120
*	heartbeat_interval_seconds: 20
*	lease acceptance timeout (wait for AckLease): 30
*	cancel deadline: 30
*	maximum job runtime: per job (e.g., 60 min) + safety buffer

## Retry and Attempt Rules

Retries are orchestrator-driven.
*	Orchestrator decides if a failure is retryable.
*	Runner reports only:
*	exit code
*	summary
*	artifacts/logs

The protocol supports retries by creating a new attempt and a new lease.

## Security Considerations
*	lease_id is a capability token; treat it as a secret.
*	Runners must not log or expose lease_id or cancel_token.
*	Control-plane endpoints must authenticate runners (mTLS, signed tokens, or OIDC).
*	Artifacts and logs are untrusted inputs; downstream analysis must sanitize.

## Failure Modes and Expected Behavior

**Runner crash**
*	No heartbeat → lease expires → job returns to queued state (new lease possible)

**Network partition**
*	Heartbeats fail → lease may expire
*	Runner may continue execution but completion will be rejected as stale

**Duplicate dispatch**
*	Multiple runners may receive the same job
*	Only the runner with the currently valid lease can finalize it

**Orchestrator restart**
*	State is recovered from DB
*	Existing leases remain valid if within TTL, otherwise expire naturally

## Minimal Compliance Checklist

A runner implementation is considered protocol-compliant if it:
*	obtains a lease before execution
*	sends AckLease
*	heartbeats at the configured interval
*	uploads logs/artifacts and references them in completion
*	sends Complete exactly once per lease
*	supports cancellation (stop + CancelAck)
*	stops work on StaleLease

## Related Documents
*	overview.md
*	components.md
*	data-plane.md
*	state-machines.md
*	security-model.md
*	reference/runner-messages.md (future: canonical schemas)