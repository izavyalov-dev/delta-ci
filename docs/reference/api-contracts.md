# API Contracts

This document describes the **external and internal API contracts** of Delta CI.

The purpose of this document is to define **what APIs exist, what they guarantee, and what they do not guarantee**.  
This is a contract document — not an implementation guide.

If an API behavior is not documented here, it must be considered undefined.

---

## Scope

This document covers:

- public-facing APIs (VCS integration, UI/CLI)
- internal control plane APIs
- high-level request/response semantics
- idempotency and error handling rules

It does **not** define:
- runner protocol messages (see `runner-messages.md`)
- database schemas
- authentication implementation details

---

## API Design Principles

All Delta CI APIs follow these principles:

- explicit and versioned
- idempotent where possible
- side-effect-aware
- stateless at the transport level
- aligned with state machines

APIs expose **intent**, not internal mechanics.

---

## API Versioning

All APIs are versioned.

Example:
```
/api/v1/…
```

Rules:
- breaking changes require a new version
- old versions are supported for a defined deprecation period
- versioning applies to both public and internal APIs

---

## Authentication and Authorization

All API requests must be authenticated.

High-level requirements:
- human users: OAuth/OIDC
- services/runners: short-lived credentials (OIDC or signed tokens)
- all requests are authorized via RBAC or scoped tokens

Authentication details are implementation-specific and not part of this contract.

---

## Public APIs

### Webhook Ingestion API

Used by VCS providers to notify Delta CI of repository events.

**Endpoint**
```
POST /api/v1/webhooks/{provider}
```

**Responsibilities**
- validate webhook signature
- normalize event payload
- enforce idempotency
- enqueue run creation

**Idempotency**
- duplicate events must not create duplicate runs
- idempotency key derived from:
  - repository ID
  - commit SHA
  - event type
  - PR number (if applicable)

**Responses**
- `2xx` — event accepted
- `4xx` — invalid payload or signature
- `5xx` — temporary failure (VCS may retry)

**Provider Details**
- GitHub integration specifics are defined in `reference/vcs-github.md`.

---

### Run Management API

Used by UI and CLI.

#### Create Run (Manual Trigger)
```
POST /api/v1/runs
```

**Request**
```json
{
  "repo_id": "repo_123",
  "ref": "refs/heads/main",
  "reason": "manual"
}
```

**Semantics**
* creates a new run attempt
* does not override existing runs
* subject to policy checks

### Get Run
```
GET /api/v1/runs/{run_id}
```

Returns:
*	run status
*	jobs and attempts
*	timestamps
*	plan explainability metadata (source, explain, skipped jobs)
*	links to artifacts
*	lease IDs are never returned; artifacts is an array (empty when none)
*	artifact URIs are untrusted input and must be sanitized before use
*	failure explanations are advisory and may be empty
*	failure explanations include rule versions and classification signals when available

Example response:
```json
{
  "run": {
    "id": "run_456",
    "repo_id": "repo_123",
    "ref": "refs/heads/main",
    "commit_sha": "abc123",
    "state": "SUCCEEDED",
    "created_at": "2026-01-12T08:00:00Z",
    "updated_at": "2026-01-12T08:05:00Z"
  },
  "jobs": [
    {
      "job": {
        "id": "job_123",
        "run_id": "run_456",
        "name": "build",
        "required": true,
        "state": "SUCCEEDED",
        "attempt_count": 1,
        "reason": "go build triggered by code change",
        "created_at": "2026-01-12T08:00:00Z",
        "updated_at": "2026-01-12T08:05:00Z"
      },
      "attempts": [
        {
          "id": "attempt_abc",
          "job_id": "job_123",
          "attempt_number": 1,
          "state": "SUCCEEDED",
          "created_at": "2026-01-12T08:00:00Z",
          "updated_at": "2026-01-12T08:05:00Z",
          "started_at": "2026-01-12T08:01:00Z",
          "completed_at": "2026-01-12T08:05:00Z"
        }
      ],
      "artifacts": [
        {
          "id": 1,
          "job_attempt_id": "attempt_abc",
          "type": "log",
          "uri": "s3://delta-ci-artifacts/runs/run_456/jobs/job_123/log.txt",
          "created_at": "2026-01-12T08:05:00Z"
        }
      ],
      "failure_explanations": [
        {
          "id": 1,
          "job_attempt_id": "attempt_abc",
          "category": "USER",
          "summary": "Test step failed (exit code 1).",
          "confidence": "MEDIUM",
          "rule_version": "v2",
          "signals": {
            "exit_code": 1,
            "attempt_number": 1,
            "duration_seconds": 240,
            "cache_events": [
              {
                "type": "deps",
                "key": "go:deps:...",
                "hit": false,
                "read_only": true
              }
            ],
            "artifact_types": ["log"],
            "has_log": true
          },
          "details": "Observed: exit status 1 | Log: s3://delta-ci-artifacts/runs/run_456/jobs/job_123/log.txt",
          "created_at": "2026-01-12T08:05:01Z"
        }
      ]
    }
  ],
  "plan": {
    "recipe_source": "discovery",
    "fingerprint": "sha256:...",
    "explain": "diff-aware planner v1: discovered go.mod; changed paths: main.go; code change",
    "skipped_jobs": []
  }
}
```

### Cancel Run
```
POST /api/v1/runs/{run_id}/cancel
```

**Semantics**
*	transitions run to CANCEL_REQUESTED
*	emits cancel signals to active jobs
*	idempotent

### Rerun
```
POST /api/v1/runs/{run_id}/rerun
```

**Semantics**
*	creates a new run attempt
*	previous attempts remain immutable
*	may optionally filter jobs
*	idempotent when `Idempotency-Key` header is provided

## Status Reporting API

Used internally by the Status Reporter to communicate with VCS providers.

This API is outbound-only from Delta CI.

Semantics:
*	reflect orchestrator state exactly
*	updates must be idempotent
*	partial updates must be tolerated

VCS-specific details are abstracted behind provider adapters.
GitHub reporting behavior is documented in `reference/vcs-github.md`.

## Internal Control Plane APIs

These APIs are not exposed publicly but are part of the system contract.

### Planning API

Used by the Orchestrator to request a plan.

```
POST /api/v1/internal/plans
```

**Request**
```json
{
  "run_id": "run_456",
  "repo_id": "repo_123",
  "commit_sha": "abc123",
  "diff_summary": { }
}
```

**Response**
```json
{
  "plan_id": "plan_789",
  "jobs": [],
  "explain": "Why these jobs were selected"
}
```

**Semantics**
*	must be deterministic
*	must not enqueue jobs
*	must not mutate state

### Failure Analysis API

Used by the Orchestrator after job failure.
```
POST /api/v1/internal/failures/analyze
```

Request
```json
{
  "run_id": "run_456",
  "job_id": "job_123",
  "artifacts": [],
  "logs_ref": "..."
}
```

**Semantics**
*	read-only analysis
*	advisory output only
*	may invoke AI under policy constraints

## Runner-Facing APIs

Runners interact with the control plane only via:
*	runner protocol endpoints
*	artifact upload endpoints

These are defined in:
*	runner-messages.md
*	runner-protocol.md

Runners do not call general-purpose APIs.

## Error Handling

### Error Structure

All API errors use a structured format.
```json
{
  "error": {
    "code": "INVALID_STATE",
    "message": "Cannot cancel a completed run",
    "details": {}
  }
}
```

### Error Categories
*	INVALID_REQUEST — malformed or invalid input
*	UNAUTHORIZED — authentication failure
*	FORBIDDEN — authorization failure
*	NOT_FOUND — missing resource
*	INVALID_STATE — illegal state transition
*	INTERNAL_ERROR — unexpected failure

## Idempotency Guarantees
The following operations must be idempotent:
*	webhook ingestion
*	run cancellation
*	status updates
*	rerun requests (when replayed)

Idempotency violations are critical bugs.

## Consistency Guarantees

APIs guarantee:
*	monotonic state progression
*	no rollback of terminal states
*	eventual consistency for read APIs

Strong consistency is enforced for state transitions.

## Non-Goals

These APIs do not aim to:
*	expose internal database models
*	provide real-time streaming interfaces
*	support ad-hoc scripting against internal state

## Related Documents
*	reference/runner-messages.md
*	architecture/control-plane.md
*	architecture/state-machines.md
*	architecture/runner-protocol.md

## Summary

Delta CI APIs are conservative, explicit, and state-driven.

They exist to express intent and transitions, not to leak internal implementation details.
Clear contracts are critical to correctness, security, and long-term maintainability.
