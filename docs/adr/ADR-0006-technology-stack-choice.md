# ADR-0006: Technology Stack Choice

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Delta CI is an infrastructure-heavy system that must:

- execute untrusted code safely
- maintain strict state machine correctness
- remain operable under partial failure
- be self-host friendly
- scale incrementally
- attract open-source contributors

Early technology choices strongly influence:
- correctness guarantees
- operability and debuggability
- contributor velocity
- long-term sustainability

Given Delta CI’s goals (diff-aware planning, lease-based execution, explicit state machines), the technology stack must prioritize **clarity and reliability over novelty**.

---

## Decision

Delta CI adopts the following core technology stack:

- **Go** for control plane services and runners
- **PostgreSQL** as the authoritative state database
- **At-least-once queue** (Postgres-backed initially, Redis later)
- **S3-compatible object storage** for artifacts
- **HTTP + JSON** for APIs and runner protocol
- **Prometheus + structured logs** for observability
- **OpenTelemetry** for tracing (optional initially)
- **Container-first deployment**, Kubernetes-friendly

This stack is considered the **baseline** for all initial development.

---

## Detailed Decisions

### Control Plane Language: Go

Go is used for:
- API Gateway
- Orchestrator
- Planner
- Failure Analyzer
- Status Reporter

**Rationale**
- strong concurrency primitives
- predictable performance
- small static binaries
- excellent observability ecosystem
- common in infrastructure tooling
- easy self-hosting

Framework-heavy approaches are intentionally avoided.

---

### Runner Language: Go

The primary runner implementation is in Go.

**Rationale**
- shared protocol implementation
- cross-compilation
- strong process control
- minimal runtime dependencies

Alternative runners (e.g., Rust, Python) may be added later but must strictly comply with the runner protocol.

---

### Database: PostgreSQL

PostgreSQL is the single source of truth for:
- runs
- jobs and attempts
- leases and heartbeats
- state transitions
- audit events

**Rationale**
- strong transactional guarantees
- mature tooling
- easy self-hosting and managed options
- predictable failure behavior

NoSQL databases are explicitly rejected for authoritative state.

---

### Queue: At-Least-Once Delivery

Initial implementation:
- Postgres-backed queue (e.g., `SELECT … FOR UPDATE SKIP LOCKED`)

Future options:
- Redis Streams
- managed queue services (abstracted)

**Rationale**
- at-least-once semantics match lease model
- simpler failure handling
- fewer components during dogfooding

Queue choice must not leak into orchestration logic.

---

### Artifact Storage: S3-Compatible Object Store

Used for:
- logs
- test reports
- build artifacts (optional)

**Rationale**
- scalable
- cost-effective
- widely supported
- self-hostable (e.g., MinIO)

Artifacts are always treated as untrusted input.

---

### APIs and Protocols

- HTTP + JSON for all APIs
- Versioned endpoints (`/api/v1`)
- Runner protocol defined independently of transport

**Rationale**
- debuggability
- easy inspection and testing
- low barrier to entry for contributors

Protobuf may be introduced later if required by scale.

---

### Authentication and Secrets

- OIDC where possible
- signed short-lived tokens for runners
- external secrets manager recommended

Secrets are:
- never persisted
- never logged
- never sent to AI systems

---

### Observability

- structured JSON logs to stdout/stderr
- Prometheus-compatible metrics
- OpenTelemetry for tracing (optional early)

**Rationale**
- industry standard tooling
- low operational friction
- strong ecosystem support

---

### Containerization and Deployment

- Docker-compatible images
- minimal base images
- immutable tags

Kubernetes is supported but not required.

No custom operators are required initially.

---

## Non-Goals

This technology stack intentionally avoids:

- service meshes
- heavy workflow engines
- DSLs for pipelines
- proprietary managed-only dependencies
- tight coupling to any cloud provider

These are considered accidental complexity for the current scope.

---

## Consequences

### Positive

- high reliability and predictability
- strong alignment with infra best practices
- easy self-hosting
- approachable for contributors
- clear operational model

---

### Negative

- fewer “bleeding-edge” features
- slower experimentation with novel runtimes
- some performance optimizations deferred

These tradeoffs are accepted.

---

## Alternatives Considered

### JVM-based Stack
Rejected due to:
- heavier runtime
- slower startup
- higher operational overhead

---

### Event-Sourced / NoSQL Core
Rejected due to:
- increased complexity
- harder reasoning for correctness
- weaker transactional guarantees

---

### Serverless-First Architecture
Rejected due to:
- loss of execution control
- difficulty enforcing leases and heartbeats
- vendor lock-in risk

---

## Relationship to Other ADRs

- Supports: ADR-0004 (Control Plane vs Data Plane)
- Supports: ADR-0005 (Runner Lease Model)
- Informed by: ADR-0001 (Project Scope and Goals)

---

## Summary

The chosen technology stack reflects Delta CI’s core values:

- correctness over cleverness
- explicit state over implicit behavior
- boring infrastructure over fragile innovation

This stack is the foundation upon which Delta CI is built and evolved.