# Architecture Overview

This document provides a high-level architectural overview of **Delta CI**.

It explains the main system boundaries, responsibilities, and execution flow without diving into protocol or implementation details.  
The goal is to give contributors and reviewers a clear mental model of how the system works.

---

## Architectural Goals

Delta CI is designed to:

- React to **code changes**, not static pipelines
- Minimize unnecessary execution and compute waste
- Provide fast, high-signal feedback to developers
- Remain secure when executing untrusted code
- Scale horizontally with predictable behavior

These goals strongly influence the system architecture.

---

## High-Level System Model

Delta CI follows a **Control Plane / Data Plane** architecture.

### Control Plane
Responsible for:
- receiving events (PRs, pushes, manual triggers)
- deciding *what* should run and *why*
- tracking state and progress
- reporting results back to the VCS
- explaining failures and proposing fixes

### Data Plane
Responsible for:
- executing jobs
- isolating untrusted code
- producing logs and artifacts
- reporting execution results

This separation ensures that execution is scalable, isolated, and recoverable, while orchestration remains deterministic and auditable.

---

## Core Components

### Orchestrator
The Orchestrator is the central authority of the system.

Responsibilities:
- create and track runs and job attempts
- enforce state transitions
- enqueue jobs for execution
- handle retries, cancellations, and timeouts
- finalize run outcomes

The Orchestrator does **not** execute user code.

---

### Planner
The Planner determines **what should run** for a given change.

Inputs:
- repository metadata
- changed files (diff)
- previous successful build recipes
- explicit configuration (if present)

Outputs:
- a concrete execution plan (jobs, dependencies, caches, artifacts)

The Planner may use AI assistance, but all plans must be explainable and policy-compliant.

---

### Failure Analyzer
The Failure Analyzer interprets job failures.

Responsibilities:
- collect logs and artifacts
- classify failure types
- explain failures in human-readable form
- optionally propose safe fixes or patches

The Failure Analyzer never applies code changes automatically.

---

### Status Reporter
The Status Reporter is responsible for external feedback.

Responsibilities:
- update commit and PR status checks
- publish annotations and summaries
- post comments when necessary

It is the only component allowed to communicate results back to the VCS.

---

### Runner Controller
The Runner Controller manages execution capacity.

Responsibilities:
- provision and scale runners
- match jobs to runner capabilities
- enforce execution limits and quotas

It does not understand job semantics.

---

### Runner
A Runner is an ephemeral execution environment.

Characteristics:
- short-lived
- single-job ownership
- isolated filesystem and network
- no long-lived secrets

Runners communicate with the Orchestrator using a lease-based protocol.

---

## Execution Flow (Simplified)

1. A VCS event (PR, push) triggers a new run
2. The Orchestrator creates a run record
3. The Planner generates an execution plan
4. Jobs are enqueued for execution
5. Runners lease and execute jobs
6. Logs and artifacts are collected
7. Results are reported back to the VCS
8. The run is finalized

Each step is designed to be idempotent and recoverable.

---

## State and Persistence

Delta CI persists:
- runs and attempts
- job states and timings
- execution plans
- artifacts and logs
- derived build recipes

Persistent state allows:
- recovery from crashes
- accurate retries
- explainability
- historical analysis

---

## Failure Model

The architecture assumes that:
- runners can disappear at any time
- network calls can fail
- duplicate events can occur
- partial execution is normal

The system is designed to **recover by design**, not by manual intervention.

---

## Security Boundaries

Key security assumptions:
- runners are untrusted
- PR code may be malicious
- secrets must never leak into forked PRs
- artifacts must be treated as untrusted input

Security enforcement happens at the architecture level, not as an afterthought.

---

## Non-Goals

This architecture explicitly does not aim to:
- replace all CI systems immediately
- perform autonomous deployments
- hide complexity from operators
- optimize for every possible language or toolchain

Delta CI prioritizes correctness, clarity, and signal quality over completeness.

---

## Related Documents

- components.md
- control-plane.md
- data-plane.md
- runner-protocol.md
- state-machines.md
- security-model.md
- ADRs in `docs/adr/`

---

## Summary

Delta CI is built as a deterministic orchestration system with ephemeral execution.

By separating decision-making from execution and grounding behavior in explicit state machines and protocols, the architecture aims to provide predictable, secure, and explainable CI behavior—even in the presence of failure.