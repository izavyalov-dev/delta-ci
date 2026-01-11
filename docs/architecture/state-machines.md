# State Machines

This document defines the **authoritative state machines** for Delta CI.

These state machines are the foundation for correctness. They determine:
- what transitions are allowed
- which component is responsible for each transition
- how retries, timeouts, cancellations, and crashes are handled

If an implementation deviates from these rules, behavior becomes undefined.

---

## Scope

This document covers:
- **Run** lifecycle
- **Job** lifecycle
- **Lease** lifecycle (runner execution rights)
- retry and cancellation semantics at the state-machine level

It does not define:
- the runner protocol details (see `runner-protocol.md`)
- job spec formats (see `reference/*`)

---

## Definitions

- **Run**: A single CI evaluation of a commit/PR trigger, consisting of one or more jobs.
- **Attempt**: A new run instance for the same commit/trigger, created by rerun.
- **Job**: A unit of work within a run (build, test, lint, etc.).
- **Job Attempt**: A retry of a job (same logical job, new execution attempt).
- **Lease**: Exclusive right for a runner to execute a job attempt.

---

## Run State Machine

### States

- `CREATED` — run record created (before planning begins)
- `PLANNING` — planner is producing an execution plan
- `PLAN_FAILED` — planning failed (error/timeout)
- `QUEUED` — jobs created and enqueued, waiting for execution
- `RUNNING` — at least one job attempt is active or completed
- `CANCEL_REQUESTED` — cancel requested; jobs may still be active
- `SUCCESS` — all required jobs succeeded
- `FAILED` — any required job failed, or plan failed
- `CANCELED` — run canceled before completion
- `REPORTED` — external statuses finalized (VCS updated)
- `TIMEOUT` — run exceeded max runtime (treated as failed for reporting)

### Transitions

1. `CREATED -> PLANNING`  
   **Owner:** Orchestrator  
   Trigger: run created by webhook/manual command

2. `PLANNING -> QUEUED`  
   **Owner:** Orchestrator  
   Trigger: planner returns a plan, jobs created

3. `PLANNING -> PLAN_FAILED`  
   **Owner:** Orchestrator  
   Trigger: planner error or timeout

4. `PLAN_FAILED -> FAILED`  
   **Owner:** Orchestrator  
   Trigger: run finalization

5. `QUEUED -> RUNNING`  
   **Owner:** Orchestrator  
   Trigger: first job attempt becomes active (leased/starting/running)

6. `RUNNING -> SUCCESS`  
   **Owner:** Orchestrator  
   Condition: all **required** jobs succeeded (allow-failures may exist)

7. `RUNNING -> FAILED`  
   **Owner:** Orchestrator  
   Condition: any required job reaches terminal failure with no retries left

8. `RUNNING|QUEUED -> CANCEL_REQUESTED`  
   **Owner:** Orchestrator  
   Trigger: user cancel or superseded run policy

9. `CANCEL_REQUESTED -> CANCELED`  
   **Owner:** Orchestrator  
   Condition: all active jobs terminated or cancel deadlines exceeded

10. `RUNNING -> TIMEOUT`  
    **Owner:** Orchestrator  
    Trigger: max runtime exceeded

11. `SUCCESS|FAILED|CANCELED|TIMEOUT -> REPORTED`  
    **Owner:** Status Reporter (invoked by Orchestrator)  
    Trigger: final statuses/checks published

12. `REPORTED -> (terminal)`  
    End state

### Run Finalization Rules

- A run is **SUCCESS** only if all required jobs are **SUCCEEDED**.
- A run is **FAILED** if:
  - planning failed, or
  - any required job is in terminal failure, or
  - the run timed out.
- A run is **CANCELED** only if cancellation was requested and the system either:
  - received cancel acknowledgements, or
  - exceeded cancel deadlines and forced finalization.

---

## Job State Machine

Each job has **attempts**. The job state reflects the current attempt and overall resolution.

### Job States (per attempt)

- `CREATED` — attempt created
- `QUEUED` — attempt ready to be leased
- `LEASED` — lease granted to a runner
- `STARTING` — runner acknowledged lease, preparing env
- `RUNNING` — steps executing
- `UPLOADING` — logs/artifacts uploading
- `SUCCEEDED` — completed successfully
- `FAILED` — completed with failure (non-zero / test failure / timeout)
- `CANCEL_REQUESTED` — cancel requested for this attempt
- `CANCELED` — canceled and acknowledged (or forced)
- `TIMED_OUT` — exceeded job timeout (often treated as failed)
- `STALE` — attempt lost lease; results ignored (implementation detail)

### Transitions

1. `CREATED -> QUEUED`  
   **Owner:** Orchestrator  
   Trigger: job attempt created and published for dispatch

2. `QUEUED -> LEASED`  
   **Owner:** Orchestrator  
   Trigger: lease granted (either by queue lease or API lease)

3. `LEASED -> STARTING`  
   **Owner:** Orchestrator  
   Trigger: `AckLease` received

4. `STARTING -> RUNNING`  
   **Owner:** Orchestrator (driven by runner progress/first heartbeat)  
   Trigger: first heartbeat or explicit “running” report

5. `RUNNING -> UPLOADING`  
   **Owner:** Runner (observed by Orchestrator)  
   Trigger: runner indicates completion and begins artifact upload (optional)

6. `UPLOADING -> SUCCEEDED|FAILED`  
   **Owner:** Orchestrator  
   Trigger: `Complete` accepted

7. `LEASED|STARTING|RUNNING -> QUEUED` (lease expired)  
   **Owner:** Orchestrator  
   Trigger: lease TTL expired without heartbeat; attempt considered lost and retried if allowed

8. `RUNNING -> TIMED_OUT`  
   **Owner:** Orchestrator  
   Trigger: job timeout exceeded

9. `QUEUED|LEASED|STARTING|RUNNING -> CANCEL_REQUESTED`  
   **Owner:** Orchestrator  
   Trigger: run cancellation or manual cancel job

10. `CANCEL_REQUESTED -> CANCELED`  
    **Owner:** Orchestrator  
    Trigger: `CancelAck` accepted, or cancel deadline exceeded (forced)

11. `FAILED|TIMED_OUT -> QUEUED` (retry)  
    **Owner:** Orchestrator  
    Condition: failure is retryable AND attempts remaining

12. `SUCCEEDED|FAILED|CANCELED`  
    Terminal states for the attempt

### Job Attempt vs Job (logical) Resolution

A logical job (e.g., "unit-tests") is:
- **SUCCEEDED** when one attempt succeeds
- **FAILED** when the latest attempt fails and no retries remain
- **CANCELED** when run/job cancellation is finalized

---

## Lease State Machine

Leases grant exclusive execution rights.

### Lease States

- `GRANTED` — lease issued to a runner (not yet acknowledged)
- `ACTIVE` — runner acknowledged lease and is expected to heartbeat
- `EXPIRED` — TTL elapsed without valid heartbeat
- `REVOKED` — orchestrator invalidated lease (rare; e.g., explicit revocation)
- `COMPLETED` — lease ended via accepted completion
- `CANCELED` — lease ended via accepted cancellation

### Transitions

1. `GRANTED -> ACTIVE`  
   Trigger: `AckLease`

2. `GRANTED|ACTIVE -> EXPIRED`  
   Trigger: TTL exceeded without heartbeat

3. `ACTIVE -> COMPLETED`  
   Trigger: `Complete` accepted (valid lease)

4. `ACTIVE -> CANCELED`  
   Trigger: `CancelAck` accepted (valid lease + cancel requested)

5. `GRANTED|ACTIVE -> REVOKED`  
   Trigger: orchestrator revocation (optional feature)

### Lease Fencing Rules

- Only one lease can be **ACTIVE** for a given job attempt.
- `Complete` and `CancelAck` must be accepted only if the lease is **ACTIVE**.
- Any message referencing `EXPIRED/REVOKED` lease must be treated as **stale** and must not mutate job final state.

---

## Cancellation Semantics

Cancellation is two-phase:

1. **Request**  
   Orchestrator transitions run/jobs to `CANCEL_REQUESTED` and sends cancel signals.

2. **Finalize**  
   Orchestrator finalizes jobs as `CANCELED` when:
   - it receives `CancelAck`, or
   - cancel deadline is exceeded (forced cancel)

Queued jobs without an active lease may be finalized immediately after cancellation.

### Forced Cancel

Forced cancel exists to prevent stuck runs and resource leaks.

Rules:
- Forced cancel must still upload and preserve any available logs/metadata.
- Forced cancel must not accept late `Complete` from stale leases.

---

## Retry Semantics

Retries are orchestrator-driven.

### Retry Eligibility

A failure is retryable based on classification (rules-based recommended):
- transient network/infra errors
- runner startup failures
- cache/registry fetch issues
- flaky tests (if detected)

Non-retryable examples:
- compilation errors
- deterministic test failures
- lint/style violations

### Retry Transition

When retrying:
- create a new job attempt with incremented `attempt_index`
- publish it to dispatch queue
- previous attempt becomes terminal `FAILED` (or `TIMED_OUT`) but does not define job resolution if retries remain

---

## Timeouts

Delta CI enforces timeouts at multiple levels:

- **Lease TTL** (liveness)
- **Job timeout** (max execution time)
- **Run timeout** (max pipeline time)
- **Cancel deadline** (graceful stop window)

Timeout handling must be explicit in Orchestrator state transitions.

---

## Idempotency Rules

Orchestrator must treat these operations as idempotent:
- receiving duplicate VCS webhooks for the same commit/PR event
- receiving duplicate runner messages (heartbeats, completes) due to retries
- re-processing queue deliveries (at-least-once semantics)

Recommended strategy:
- use `(lease_id, message_type, sequence_no)` or `(lease_id, ts)` + monotonic checks
- store job events to audit and dedupe

---

## Related Documents

- `overview.md`
- `components.md`
- `runner-protocol.md`
- `security-model.md`
- `reference/terminology.md`

---

## Summary

Delta CI relies on explicit state machines to remain predictable under failure.

- The Orchestrator owns state transitions.
- Runners provide execution and best-effort progress signals.
- Leases fence execution rights and prevent double-finalization.
- Cancellation and retries are orchestrator-driven and time-bounded.

These rules enable safe, scalable CI execution in hostile and failure-prone environments.
