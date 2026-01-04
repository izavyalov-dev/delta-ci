# ADR-0004: Control Plane vs Data Plane Separation

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Delta CI executes **untrusted user code** while also maintaining **authoritative system state**.

Mixing execution logic with orchestration logic creates significant risks:
- compromised runners influencing system state
- difficulty reasoning about failures
- poor scalability
- unclear ownership of decisions
- security boundary erosion

Many CI systems blur these responsibilities, leading to:
- long-lived workers with implicit state
- side effects during execution
- fragile recovery behavior
- limited auditability

To achieve correctness, security, and explainability, Delta CI must enforce strict responsibility boundaries.

---

## Decision

Delta CI adopts a **strict Control Plane / Data Plane architecture**.

- The **Control Plane** owns decisions, state, and orchestration.
- The **Data Plane** executes jobs and produces artifacts.
- The two planes communicate only through explicit, versioned protocols.

No component may cross these boundaries.

---

## Control Plane Responsibilities

The Control Plane is **trusted** and responsible for:

- receiving external events
- creating and managing runs and attempts
- planning execution (diff-aware)
- enforcing state machines
- issuing leases
- deciding retries, cancellations, and timeouts
- persisting authoritative state
- reporting results externally

The Control Plane:
- never executes user code
- never trusts runner-provided state blindly
- remains deterministic and auditable

---

## Data Plane Responsibilities

The Data Plane is **untrusted** and responsible for:

- executing a single job attempt per runner
- providing isolated execution environments
- producing logs and artifacts
- reporting progress and results via protocol

The Data Plane:
- never makes orchestration decisions
- never mutates authoritative state
- never communicates with VCS systems
- may fail or disappear at any time

---

## Boundary Enforcement

The separation is enforced through:

- explicit APIs and protocols
- lease-based execution fencing
- immutable artifacts
- centralized state transitions
- strict authentication between planes

Any direct coupling is considered a design violation.

---

## Rationale

### 1. Security

Untrusted code must not influence:
- retries
- job finalization
- run outcomes
- secret access

Separation ensures that even a compromised runner:
- cannot finalize jobs incorrectly
- cannot escalate privileges
- cannot mutate system state

---

### 2. Correctness Under Failure

Execution environments are unreliable.

By separating concerns:
- runner crashes do not corrupt state
- retries are deterministic
- cancellation is enforceable
- recovery is straightforward

State machines remain authoritative.

---

### 3. Scalability

Control Plane:
- scales with orchestration complexity
- remains lightweight and deterministic

Data Plane:
- scales horizontally with workload
- can be replaced, throttled, or isolated independently

This enables predictable scaling behavior.

---

### 4. Explainability and Auditability

When decisions are centralized:
- every transition is logged
- every decision is explainable
- behavior can be reconstructed post hoc

This is essential for trust and debugging.

---

## Consequences

### Positive

- clear ownership of responsibilities
- strong security boundaries
- simpler reasoning about failures
- safer execution of untrusted code
- improved auditability

---

### Negative

- more components to reason about
- increased initial complexity
- need for explicit protocols and state machines

These tradeoffs are accepted for long-term correctness.

---

## Alternatives Considered

### Monolithic Worker Model

Rejected because:
- mixes execution with orchestration
- increases blast radius of compromise
- complicates recovery
- hides state transitions

---

### Long-Lived Stateful Workers

Rejected because:
- implicit state leads to non-determinism
- difficult to debug
- poor isolation guarantees

---

### Fully Stateless Execution Without Orchestrator

Rejected because:
- lacks coordination
- cannot support retries or cancellation
- cannot provide explainability

---

## Relationship to Other Decisions

- Enables: ADR-0005 (Runner Lease Model)
- Supports: ADR-0002 (Diff-Aware CI)
- Enforced by: Runner Protocol and State Machines

---

## Summary

Strict separation between control plane and data plane is a **non-negotiable architectural constraint** in Delta CI.

It is the foundation that enables:
- safe execution
- deterministic behavior
- explainable decisions
- reliable recovery

All future design decisions must preserve this separation.