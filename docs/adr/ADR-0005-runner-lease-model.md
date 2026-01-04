# ADR-0005: Runner Lease Model

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Delta CI executes jobs on **ephemeral, untrusted runners** that may:
- crash or disappear
- lose network connectivity
- receive duplicate dispatch messages
- behave incorrectly or maliciously

At the same time, the system must guarantee:
- each job attempt is finalized exactly once
- late or duplicate results do not corrupt state
- retries and cancellations behave deterministically
- execution authority is clearly defined

Traditional “worker picks job and reports result” models fail under these conditions, especially with at-least-once delivery and unreliable workers.

---

## Decision

Delta CI adopts a **lease-based execution model** for runner-job interaction.

A **Lease** grants a runner the exclusive, time-bounded right to execute and finalize a specific job attempt.

Only a runner holding an **active lease** may:
- execute the job attempt
- report heartbeats
- finalize the attempt via completion or cancellation

The lease acts as a **fencing token**.

---

## Lease Properties

A lease has the following properties:

- **Exclusive**  
  Only one active lease may exist for a job attempt at any time.

- **Time-bounded (TTL)**  
  A lease expires automatically if not renewed.

- **Opaque and unguessable**  
  Identified by a `lease_id` treated as a capability token.

- **Revocable by expiration**  
  The orchestrator never needs to forcibly kill runners to reclaim authority.

---

## Lease Lifecycle

1. Orchestrator grants a lease to a runner
2. Runner acknowledges the lease (`AckLease`)
3. Runner sends periodic heartbeats
4. Orchestrator renews the lease on each heartbeat
5. Runner completes or cancels the job
6. Orchestrator finalizes the job attempt
7. Lease transitions to a terminal state

Lease expiration implicitly revokes execution rights.

---

## Fencing Semantics

The lease enforces fencing:

- only messages referencing the **current active lease** are accepted
- stale leases cannot mutate job state
- late completions are safely rejected
- duplicate dispatch does not cause double execution

This prevents:
- double-finalization
- split-brain execution
- state corruption from retries

---

## Cancellation Handling

Cancellation is integrated into the lease model.

Rules:
- cancellation is requested by the orchestrator
- runner receives a cancel signal tied to the lease
- runner must acknowledge cancellation
- orchestrator may forcibly finalize after a deadline

A canceled lease cannot be completed successfully.

---

## Failure Handling

### Runner Crash
- no heartbeats
- lease expires
- job attempt becomes eligible for retry

### Network Partition
- heartbeats fail
- lease may expire
- completion may be rejected as stale

### Orchestrator Restart
- state recovered from persistence
- active leases remain valid until TTL
- no special coordination required

---

## Rationale

### 1. Correctness Under At-Least-Once Delivery

Queues and networks often deliver messages more than once.

The lease model ensures:
- multiple runners may *see* a job
- only one runner may *finalize* it

---

### 2. Simple Recovery Model

Lease expiration is sufficient to recover from:
- crashes
- hangs
- network issues

No complex “worker health” tracking is required.

---

### 3. Clear Ownership of Authority

The lease makes execution authority explicit:
- possession of `lease_id` == right to execute
- loss of lease == loss of authority

This simplifies reasoning and implementation.

---

### 4. Security

Leases limit blast radius:
- compromised runner cannot finalize jobs indefinitely
- execution rights expire automatically
- stale results are rejected

---

## Consequences

### Positive

- strong correctness guarantees
- simple failure recovery
- robust against duplicate messages
- explicit execution authority
- minimal coupling between runner and orchestrator

---

### Negative

- additional protocol complexity
- need for heartbeat traffic
- careful TTL tuning required

These costs are accepted.

---

## Alternatives Considered

### Implicit Ownership (Worker ID Based)
Rejected because:
- workers may restart or duplicate
- identity-based ownership is brittle
- difficult to fence stale workers

---

### Centralized Locking
Rejected because:
- hard to scale
- prone to deadlocks
- difficult to recover from partial failures

---

### Long-Lived Worker Sessions
Rejected because:
- introduce implicit state
- complicate recovery
- weaken isolation

---

## Relationship to Other Decisions

- Builds on: ADR-0004 (Control vs Data Plane)
- Enables: Robust Runner Protocol
- Supports: State Machine Design

---

## Summary

The runner lease model is the **execution safety backbone** of Delta CI.

By making execution authority:
- explicit
- time-bounded
- revocable

Delta CI achieves predictable, secure, and correct job execution even in hostile and failure-prone environments.

This decision is fundamental and non-negotiable.