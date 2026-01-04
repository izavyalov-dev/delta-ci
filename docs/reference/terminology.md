# Terminology

This document defines the **canonical terminology** used throughout Delta CI.

These terms are used consistently in documentation, code, APIs, logs, and discussions.  
If a term is used ambiguously or inconsistently, it should be treated as a documentation bug.

---

## Core Concepts

### Repository
A source code repository connected to Delta CI.

A repository may contain:
- a single project
- multiple projects (monorepo)
- optional Delta CI configuration

---

### Run
A **Run** represents a single CI evaluation triggered by an event.

Examples of triggers:
- pull request opened or updated
- push to a branch
- manual rerun

A run:
- is immutable once finalized
- consists of one or more jobs
- has a final outcome (success, failed, canceled)

---

### Run Attempt
A **Run Attempt** is a new execution instance for the same logical run trigger.

Examples:
- manual rerun after failure
- rerun after infrastructure issue

Each attempt:
- has its own jobs and state
- does not overwrite previous attempts
- is linked to the same repository and commit

---

### Job
A **Job** is a logical unit of work within a run.

Examples:
- build
- unit tests
- lint
- integration tests

A job:
- is defined by the planner or configuration
- may have multiple attempts
- resolves when one attempt succeeds or retries are exhausted

---

### Job Attempt
A **Job Attempt** is a single execution of a job.

A job attempt:
- is executed by exactly one runner
- is associated with exactly one lease
- has a terminal state (succeeded, failed, canceled, timed out)

Retries create new job attempts.

---

### Lease
A **Lease** is an exclusive right for a runner to execute a job attempt.

Properties:
- time-bounded (TTL)
- identified by a `lease_id`
- acts as a fencing token

Only the runner holding the active lease may finalize the job attempt.

---

### Runner
A **Runner** is an ephemeral execution environment.

A runner:
- executes one job attempt at a time
- is short-lived
- is isolated and untrusted
- communicates via the runner protocol

Runners do not own state.

---

### Runner Controller
The **Runner Controller** manages runner capacity.

Responsibilities:
- provisioning runners
- scaling up/down
- matching jobs to runner capabilities

It does not execute jobs or track job state.

---

### Control Plane
The **Control Plane** is the trusted part of Delta CI responsible for orchestration and state.

Includes:
- API Gateway
- Orchestrator
- Planner
- Failure Analyzer
- Status Reporter
- Database

The control plane never executes user code.

---

### Data Plane
The **Data Plane** is the untrusted execution layer.

Includes:
- runners
- execution environments
- transient execution resources

The data plane executes code but does not make decisions.

---

### Planner
The **Planner** decides what should run for a given change.

It:
- analyzes diffs
- performs discovery and impact analysis
- produces an execution plan

The planner proposes; the orchestrator enforces.

---

### Execution Plan
An **Execution Plan** is the concrete output of planning.

It defines:
- jobs to run
- dependencies between jobs
- cache usage
- artifact expectations
- job policies

Plans are deterministic and explainable.

---

### Recipe
A **Recipe** is a persisted, validated description of how a repository builds and tests.

Recipes:
- are created after successful runs
- are versioned and immutable
- are tied to a repository fingerprint
- accelerate future planning

Recipes never override explicit configuration.

---

### Repository Fingerprint
A **Repository Fingerprint** is a hash representing the structural identity of a repository.

It may include:
- build file hashes
- workspace definitions
- dependency manifests

Fingerprint changes indicate that recipes may be stale.

---

### Artifact
An **Artifact** is any output produced by a job.

Examples:
- logs
- test reports
- coverage data
- build outputs

Artifacts are immutable and treated as untrusted input.

---

### Cache
A **Cache** is reusable data stored to accelerate future jobs.

Examples:
- dependency caches
- build caches
- toolchain caches

Caches are explicitly keyed and scoped.

---

### Failure Analyzer
The **Failure Analyzer** inspects failed jobs and explains why they failed.

It may:
- classify failures
- produce explanations
- suggest fixes

It may not apply fixes automatically.

---

### Status Reporter
The **Status Reporter** publishes run results externally.

Examples:
- commit status checks
- PR comments
- annotations

It reflects orchestrator state exactly.

---

## States and Statuses

### Run Status
Terminal states of a run:
- `SUCCESS`
- `FAILED`
- `CANCELED`
- `TIMEOUT`

---

### Job Attempt Status
Terminal states of a job attempt:
- `SUCCEEDED`
- `FAILED`
- `CANCELED`
- `TIMED_OUT`

---

### Lease Status
Lifecycle states of a lease:
- `GRANTED`
- `ACTIVE`
- `EXPIRED`
- `REVOKED`
- `COMPLETED`
- `CANCELED`

---

## Events and Actions

### Heartbeat
A **Heartbeat** is a periodic message sent by a runner to prove liveness and extend its lease.

---

### Cancel Requested
A **Cancel Requested** event indicates that execution should stop gracefully.

Cancellation is two-phase and time-bounded.

---

### Complete
A **Complete** message finalizes a job attempt.

It is accepted only if the lease is active.

---

## Consistency Rules

- Terms in this document must be used consistently everywhere
- New terms must be added here before use
- Renaming terms requires documentation updates

---

## Related Documents

- architecture/overview.md
- architecture/components.md
- architecture/runner-protocol.md
- architecture/state-machines.md
- design/principles.md

---

## Summary

This document defines the shared language of Delta CI.

Clear terminology enables:
- precise reasoning
- correct implementations
- effective collaboration

Ambiguity is treated as a defect.