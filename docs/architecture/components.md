# Components

This document describes the core components of **Delta CI**, their responsibilities, and their interaction boundaries.

The goal is to clearly define **who owns what**, avoid responsibility overlap, and make the system easier to reason about, extend, and maintain.

---

## Component Model

Delta CI is composed of multiple loosely coupled components, grouped into two planes:

- **Control Plane** — decision-making, orchestration, and state
- **Data Plane** — execution and resource-intensive work

Each component has a single, well-defined responsibility.

---

## Control Plane Components

### API Gateway / BFF

**Responsibility**
- Entry point for all external interactions
- Authentication and authorization
- Request validation and routing

**Handles**
- VCS webhooks
- Web UI and CLI requests
- Manual reruns and cancellations

**Does NOT**
- Make planning decisions
- Execute jobs
- Track execution state directly

---

### Orchestrator

**Responsibility**
- System authority for runs and jobs
- State transitions and lifecycle management

**Handles**
- creating runs and attempts
- persisting run and job state
- enforcing idempotency
- enqueueing jobs
- retries, cancellations, and timeouts
- final run resolution

**Does NOT**
- inspect repository contents
- decide which jobs are needed
- execute user code

The Orchestrator is the single source of truth for system state.

---

### Planner

**Responsibility**
- Decide *what should run* for a given change

**Inputs**
- repository metadata
- diff summary
- existing build recipes
- explicit configuration (if present)

**Outputs**
- execution plan:
  - job list
  - dependencies
  - cache usage
  - artifact expectations

**Characteristics**
- deterministic first
- AI-assisted only where allowed
- explainable decisions

**Does NOT**
- enqueue jobs directly
- execute or validate jobs
- mutate repository state

---

### Failure Analyzer

**Responsibility**
- Understand and explain job failures

**Handles**
- log and artifact inspection
- error classification
- root cause analysis
- explanation generation
- safe fix suggestions

**Optional Capabilities**
- generate candidate patches
- request fix validation jobs

**Does NOT**
- apply patches automatically
- rerun pipelines by itself
- bypass policy restrictions

Human approval is always required for code changes.

---

### Status Reporter

**Responsibility**
- Communicate results to external systems

**Handles**
- commit and PR status checks
- annotations and summaries
- PR comments and explanations

**Does NOT**
- influence execution decisions
- access secrets
- modify repository state

This component is the only one allowed to talk back to the VCS.

---

## Data Plane Components

### Runner Controller

**Responsibility**
- Manage execution capacity

**Handles**
- provisioning runners
- scaling up/down
- capability-based scheduling
- enforcing quotas and limits

**Does NOT**
- understand job semantics
- interpret job results
- manage job state

---

### Runner

**Responsibility**
- Execute exactly one job at a time

**Characteristics**
- ephemeral
- isolated
- single-job ownership
- limited network access
- no long-lived credentials

**Lifecycle**
1. start
2. lease job
3. execute steps
4. upload artifacts
5. report result
6. terminate

Runners interact with the Orchestrator via a lease-based protocol.

---

## Supporting Infrastructure

### Queue

**Purpose**
- decouple orchestration from execution

**Used For**
- job dispatch
- backpressure
- retry handling
- cancel propagation

Queue semantics must support at-least-once delivery.

---

### Database

**Purpose**
- persistent system state

**Stores**
- runs and attempts
- jobs and leases
- plans and recipes
- audit events

The database is authoritative for state transitions.

---

### Artifact Store

**Purpose**
- durable storage of execution outputs

**Stores**
- logs
- test reports
- coverage data
- metadata

Artifacts are treated as untrusted input.

---

### Cache Store

**Purpose**
- reduce redundant work

**Used For**
- dependency caching
- build caching

Cache keys are derived from explicit inputs and fingerprints.

---

### Secrets Manager

**Purpose**
- secure credential handling

**Characteristics**
- short-lived credentials
- environment-based access
- strict policy enforcement

Secrets are never persisted or logged.

---

## Component Interaction Principles

- Components communicate via explicit APIs or messages
- Control plane never executes user code
- Data plane never makes orchestration decisions
- No component bypasses the Orchestrator
- All cross-boundary communication is auditable

---

## Failure Isolation

Component failures are isolated by design:
- runner crashes do not affect orchestrator state
- planner failures fail fast and safely
- queue backpressure slows execution, not planning
- artifact store outages degrade explainability, not correctness

---

## Extensibility

New components or capabilities must:
- integrate through existing boundaries
- respect state ownership
- not introduce hidden coupling

This allows Delta CI to evolve without architectural drift.

---

## Related Documents

- overview.md
- control-plane.md
- data-plane.md
- runner-protocol.md
- state-machines.md
- security-model.md

---

## Summary

Delta CI components are intentionally small, focused, and isolated.

By enforcing strict responsibility boundaries and relying on explicit protocols, the system remains predictable, debuggable, and safe even as complexity grows.