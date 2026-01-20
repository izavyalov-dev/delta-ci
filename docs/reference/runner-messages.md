# Runner Messages

This document defines the **canonical wire-level messages** used between **Runners** and the **Orchestrator** in Delta CI.

It complements `architecture/runner-protocol.md` by specifying **exact message schemas**, required fields, and validation rules.

These schemas are normative.  
If an implementation deviates, behavior is undefined.

---

## Scope

This document defines:
- message types
- required and optional fields
- field semantics
- validation rules
- common response patterns

It does **not** define:
- transport details (HTTP, gRPC, queue)
- authentication mechanisms
- database persistence

---

## Encoding and Transport

- Messages are shown in **JSON** for readability.
- Implementations may use JSON or Protobuf.
- Field names and semantics must remain consistent.

All messages must include:
- a `type`
- a unique identifier (lease/job)
- a timestamp where applicable

---

## Common Field Conventions

### Identifiers
- `run_id`, `job_id`, `lease_id`, `runner_id` are opaque strings.
- Identifiers must be globally unique within their scope.

### Timestamps
- ISO-8601 UTC strings
- Example: `2026-01-04T08:00:00Z`

### Status Enums
Enums must be strict; unknown values must be rejected.

---

## Message Types

---

## LeaseGranted

Sent by the Orchestrator when a runner is granted a lease.

```json
{
  "type": "LeaseGranted",
  "run_id": "run_456",
  "job_id": "job_123",
  "lease_id": "lease_abc",
  "lease_ttl_seconds": 120,
  "heartbeat_interval_seconds": 20,
  "max_runtime_seconds": 3600,
  "job_spec": {
    "name": "unit-tests",
    "workdir": ".",
    "image": "ghcr.io/delta-ci/runner-dotnet:9",
    "steps": [
      "dotnet restore",
      "dotnet test -c Release"
    ],
    "env": {
      "CI": "true"
    },
    "artifacts": [
      {
        "type": "junit",
        "path": "**/TestResults/*.trx"
      }
    ],
    "caches": [
      {
        "type": "deps",
        "key": "nuget:{lock_hash}",
        "paths": ["~/.nuget/packages"],
        "read_only": false
      }
    ]
  }
}
```

### Validation Rules
*	lease_id must be unguessable
*	lease_ttl_seconds > heartbeat_interval_seconds
*	job_spec.steps must be non-empty

## AckLease

Sent by the runner to acknowledge lease acceptance.

```json
{
  "type": "AckLease",
  "lease_id": "lease_abc",
  "job_id": "job_123",
  "runner_id": "runner_xyz",
  "accepted_at": "2026-01-04T08:00:05Z"
}
```

### Validation Rules
*	must reference an active lease_id
*	must be received within lease-accept timeout
*	duplicate AckLease messages are ignored

## Heartbeat

Sent periodically by the runner to extend the lease and report progress.

```json
{
  "type": "Heartbeat",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "progress": {
    "percent": 42,
    "current_step": "dotnet test",
    "step_index": 1,
    "message": "Running unit tests"
  },
  "log_cursor": {
    "bytes_sent": 1048576
  },
  "ts": "2026-01-04T08:01:10Z"
}
```

### HeartbeatAck (Response)
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

### Validation Rules
*	heartbeats for expired leases must be rejected
*	missing heartbeats result in lease expiration
*	progress fields are advisory only

## Complete

Sent by the runner when execution finishes.
```json
{
  "type": "Complete",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "status": "SUCCEEDED",
  "exit_code": 0,
  "timings": {
    "started_at": "2026-01-04T08:00:10Z",
    "finished_at": "2026-01-04T08:03:40Z"
  },
  "artifacts": [
    {
      "type": "log",
      "uri": "s3://delta-ci-artifacts/run_456/job_123/log.txt"
    },
    {
      "type": "junit",
      "uri": "s3://delta-ci-artifacts/run_456/job_123/test.trx"
    }
  ],
  "caches": [
    {
      "type": "deps",
      "key": "nuget:{lock_hash}",
      "hit": true,
      "read_only": false
    }
  ],
  "summary": "All tests passed"
}
```

### Validation Rules
*	accepted only for active leases
*	accepted only once per lease
*	late completes must be rejected as stale

## CancelRequested

Sent by the Orchestrator to request job cancellation.
```json
{
  "type": "CancelRequested",
  "lease_id": "lease_abc",
  "job_id": "job_123",
  "reason": "RUN_CANCELED",
  "deadline_seconds": 30,
  "ts": "2026-01-04T08:02:00Z"
}
```
### Validation Rules
*	may be delivered via control channel or heartbeat ack
*	runner must attempt graceful termination

## CancelAck

Sent by the runner to acknowledge cancellation.
```json
{
  "type": "CancelAck",
  "lease_id": "lease_abc",
  "runner_id": "runner_xyz",
  "final_status": "CANCELED",
  "ts": "2026-01-04T08:02:20Z",
  "artifacts": [
    {
      "type": "log",
      "uri": "s3://delta-ci-artifacts/run_456/job_123/log.partial.txt"
    }
  ],
  "summary": "Canceled during test execution"
}
```

### Validation Rules
*	accepted only if cancel was requested
*	accepted only for active leases
*	late acknowledgements may be ignored

## StaleLease

Sent by the Orchestrator when a message references an invalid lease.
```json
{
  "type": "StaleLease",
  "lease_id": "lease_abc",
  "reason": "LEASE_EXPIRED"
}
```

### Runner Behavior
*	stop execution immediately (best effort)
*	release local resources
*	do not retry or re-complete

## Status Enumerations

### Job Status
```
SUCCEEDED
FAILED
CANCELED
TIMED_OUT
```

### Cancel Reason
```
RUN_CANCELED
JOB_CANCELED
TIMEOUT
SUPERSEDED
```

### Stale Reason
```
LEASE_EXPIRED
LEASE_REVOKED
UNKNOWN_LEASE
```

## Ordering and Idempotency

Rules:
*	messages may be delivered more than once
*	orchestrator must deduplicate by (lease_id, type)
*	state transitions must be monotonic

Out-of-order messages must not corrupt state.

## Error Responses

When rejecting a message, the Orchestrator may respond with:
```json
{
  "error": {
    "code": "STALE_LEASE",
    "message": "Lease has expired",
    "lease_id": "lease_abc"
  }
}
```
Errors must not leak internal state.

## Security Requirements
*	lease_id must be treated as a secret
*	messages must be authenticated
*	runners must not log sensitive identifiers
*	message payloads must be size-limited

## Related Documents
*	architecture/runner-protocol.md
*	architecture/state-machines.md
*	architecture/security-model.md
*	reference/api-contracts.md
    
## Summary

Runner messages form the execution contract of Delta CI.

By strictly defining message schemas, validation rules, and lifecycle semantics, Delta CI ensures:
*	correct execution fencing
*	resilience under failure
*	predictable and secure job handling
