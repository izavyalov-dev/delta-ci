# Phase 0 Implementation Checklist

This document defines the **Phase 0 implementation plan** for Delta CI.

Phase 0 is about **making the architecture real**, not feature completeness.
The goal is to reach a minimal, correct, end-to-end system that can dogfood itself.

---

## Phase 0 Goals

By the end of Phase 0, Delta CI must:

- execute a real job end-to-end
- enforce lease-based execution
- persist and recover state
- be observable and debuggable
- run CI for its own repository

Performance, scale, and AI are explicitly secondary.

---

## Guiding Rules

- correctness > speed
- one repo, one runner, one job is enough
- no shortcuts around state machines
- no mocks for core logic
- if it’s not documented, don’t implement it

---

## Milestone 0.1 — Repository Skeleton

☑ Create repository structure  
☑ Set up Go module  
☑ Add root README.md  
☑ Add `docs/` with committed architecture  

Recommended structure:
```
cmd/
orchestrator/
runner/
internal/
orchestrator/
planner/
state/
protocol/
docs/
```

---

## Milestone 0.2 — Core Data Model

☐ Define DB schema for:
- runs
- jobs
- job attempts
- leases

☐ Implement migrations  
☐ Enforce state machine constraints in code  
☐ Prevent illegal transitions at persistence layer  

Success criteria:
- invalid transitions fail explicitly
- state is fully recoverable from DB

---

## Milestone 0.3 — Orchestrator Skeleton

☐ API to create a run  
☐ Create run → planning → job creation  
☐ Enqueue job attempt  
☐ Track job and lease state  
☐ Expose read-only run/job APIs  

Ignore:
- retries
- cancellation (for now)

---

## Milestone 0.4 — Runner Protocol (Minimal)

☐ Implement `LeaseGranted`  
☐ Implement `AckLease`  
☐ Implement `Heartbeat`  
☐ Implement `Complete`  
☐ Reject stale leases  

Success criteria:
- only one lease can finalize a job
- late completion is rejected
- lease expiration re-queues job

---

## Milestone 0.5 — Minimal Runner

☐ CLI-based runner in Go  
☐ Executes a single shell command  
☐ Sends heartbeats  
☐ Uploads a log artifact  
☐ Completes job correctly  

Runner constraints:
- one job only
- no retries
- no secrets
- isolated working directory

---

## Milestone 0.6 — Queue Integration

☐ Implement at-least-once job dispatch  
☐ Support duplicate delivery  
☐ Ensure lease fencing works under duplicates  

Validation:
- simulate duplicate job delivery
- ensure only one completion is accepted

---

## Milestone 0.7 — Artifact Storage

☐ S3-compatible storage integration  
☐ Upload logs  
☐ Store artifact references  
☐ Do not store logs in DB  

Failure mode:
- artifact upload failure must not corrupt state

---

## Milestone 0.8 — Observability (Minimal)

☐ Structured logs (JSON)  
☐ Prometheus metrics:
- runs
- jobs
- leases
- failures

☐ Correlate logs by run/job/lease IDs  

---

## Milestone 0.9 — Dogfooding

☐ Use Delta CI to build Delta CI  
☐ Run at least:
- build job
- test job

☐ Validate:
- lease expiration
- runner crash recovery
- orchestrator restart recovery

---

## Explicitly Out of Scope (Phase 0)

- diff-aware optimization
- AI features
- retries and cancellation policies
- multi-runner scheduling
- Kubernetes deployment
- caching beyond basics

These belong to later phases.

---

## Phase 0 Exit Criteria

Phase 0 is complete when:

- a full run executes end-to-end
- leases enforce correctness
- state survives crashes
- operators can explain what happened
- documentation matches behavior

If behavior and docs diverge, Phase 0 is not complete.

---

## Next Phases (Preview)

- Phase 1: Minimal usable CI
- Phase 2: Diff-aware planning
- Phase 3: AI-assisted understanding
- Phase 4: Scaling and hardening

Each phase requires updated docs and ADRs.

---

## Summary

Phase 0 is about **trust**.

If Delta CI cannot be trusted in the simplest case,
it cannot be trusted at scale.

Build the core right — everything else follows.
