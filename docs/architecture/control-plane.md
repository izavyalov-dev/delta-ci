# Control Plane

This document describes the **Control Plane** of Delta CI.

The control plane is responsible for **decision-making, orchestration, and state management**.  
It never executes user code and must remain deterministic, auditable, and secure.

---

## Responsibilities

The Control Plane is responsible for:

- receiving external events (VCS, UI, CLI)
- creating and managing runs and attempts
- planning execution based on diffs and configuration
- enforcing state machines and transitions
- enqueueing jobs for execution
- handling retries, cancellations, and timeouts
- coordinating failure analysis
- reporting final results to external systems

The Control Plane is the **source of truth** for system state.

---

## Core Components

### API Gateway / BFF

**Purpose**
- single entry point for external requests

**Responsibilities**
- authentication and authorization
- webhook ingestion
- request validation
- routing to internal services

**Notes**
- must be horizontally scalable
- must enforce idempotency keys for webhook events
- must not contain business logic

---

### Orchestrator

**Purpose**
- central coordinator and state authority

**Responsibilities**
- create run records
- manage run attempts
- persist job and attempt state
- enforce state machine transitions
- enqueue jobs
- decide retry eligibility
- enforce cancellation and timeout rules
- finalize runs

**Key Properties**
- stateless in memory
- fully recoverable from persistent state
- idempotent operations

**Explicitly Does NOT**
- inspect repository contents
- decide what jobs are needed
- execute or interpret job logic

---

### Planner

**Purpose**
- decide *what should run* for a given change

**Responsibilities**
- analyze repository metadata
- analyze changed files (diff)
- resolve existing build recipes
- apply explicit configuration if present
- generate an execution plan

**Planner Output**
- ordered list of jobs
- job dependencies
- cache definitions
- artifact expectations
- job policies (required / allow-failure)

**Constraints**
- plans must be explainable
- plans must be reproducible
- AI assistance is advisory, not authoritative

---

### Failure Analyzer

**Purpose**
- interpret failed executions

**Responsibilities**
- fetch and inspect logs and artifacts
- classify failures (infra vs user vs flake)
- produce human-readable explanations
- suggest remediation steps
- optionally generate candidate patches

**Constraints**
- analysis must be read-only by default
- any mutation requires explicit user approval
- must operate on sanitized inputs only

---

### Status Reporter

**Purpose**
- communicate results externally

**Responsibilities**
- update commit and PR status checks
- publish annotations
- post explanatory comments

**Constraints**
- must reflect orchestrator state exactly
- must be idempotent
- must not influence execution decisions

---

## Run Lifecycle (Control Plane View)

1. External event received
2. Idempotency check performed
3. Run record created
4. Planning initiated
5. Jobs created and enqueued
6. Job state updates received
7. Failures analyzed (if any)
8. Run finalized
9. Status reported

Each step is explicitly persisted.

---

## State Ownership

The Control Plane owns:

- run state
- job state
- attempt state
- lease state
- retry counters
- cancellation intent

No other component may mutate these states.

---

## Idempotency Model

### Why It Matters

The Control Plane must tolerate:
- duplicate webhooks
- retries from runners
- message redelivery from queues
- restarts and crashes

### Strategy

- deterministic run keys:
  - `(repo_id, commit_sha, trigger_type, pr_number)`
- immutable run attempts
- monotonic state transitions
- deduplication on `(entity_id, event_type, sequence)`

Idempotency violations are treated as bugs.

---

## Cancellation Handling

Cancellation is always initiated by the Control Plane.

Sources:
- user request
- superseded runs
- policy enforcement
- timeout expiration

Cancellation flow:
1. mark run as `CANCEL_REQUESTED`
2. mark active jobs as `CANCEL_REQUESTED`
3. emit cancel signals
4. wait for acknowledgements or deadlines
5. finalize run as `CANCELED`

Forced cancellation is allowed after deadlines.

---

## Retry Handling

Retries are orchestrator-driven.

Rules:
- retries are per-job-attempt
- retry eligibility is determined centrally
- retry count is bounded
- retries create new attempts

Runners are unaware of retry policy.

---

## Failure Handling

Control Plane assumes failure is normal.

Designed to tolerate:
- runner crashes
- partial job execution
- lost heartbeats
- planner failures
- queue backpressure

Recovery relies on:
- persistent state
- lease expiration
- explicit retries
- time-bounded cancellation

---

## Observability

Control Plane must emit:
- structured logs
- metrics for runs, jobs, leases
- state transition events
- audit records

Observability must not depend on runner behavior.

---

## Security Guarantees

The Control Plane guarantees:
- no execution of user code
- no trust in runner-provided state
- no direct access to artifacts without sanitization
- strict enforcement of secret policies

Any violation is a critical security bug.

---

## Non-Goals

The Control Plane does not:
- optimize execution performance
- understand build semantics
- auto-heal runner infrastructure
- perform deployments

These concerns belong elsewhere.

---

## Related Documents

- overview.md
- components.md
- data-plane.md
- runner-protocol.md
- state-machines.md
- security-model.md

---

## Summary

The Control Plane is the brain of Delta CI.

It is intentionally conservative, deterministic, and explicit.  
All execution is delegated, all state is enforced, and all decisions are auditable.

Correctness and predictability take precedence over speed or flexibility.