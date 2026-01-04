# Design Principles

This document defines the **core design principles** of Delta CI.

These principles act as **guardrails** for architecture, implementation, and future contributions.  
If a proposed change violates these principles, it should be treated as a design regression unless explicitly justified via an ADR.

---

## Principle 1: Deltas Over Pipelines

Delta CI is driven by **changes**, not static pipelines.

- execution decisions are based on *what changed*
- not every change requires a full rebuild
- running everything is a fallback, not a default

This principle exists to:
- reduce wasted compute
- shorten feedback loops
- improve signal-to-noise ratio

Static pipelines are supported only as an explicit opt-in.

---

## Principle 2: Deterministic Orchestration

All orchestration decisions must be:

- deterministic
- explainable
- reproducible

Given the same inputs (diff, config, state), the system must produce the same plan.

AI assistance may **suggest**, but must never:
- introduce nondeterminism
- bypass rules
- mutate state implicitly

---

## Principle 3: Control Plane Owns State

All authoritative state belongs to the **control plane**.

- runners execute, but do not decide
- planners propose, but do not enforce
- analyzers explain, but do not mutate

This separation ensures:
- correctness under failure
- clear ownership
- auditable behavior

Any state mutation outside the control plane is a bug.

---

## Principle 4: Execution Is Untrusted

Delta CI assumes that:
- user code is untrusted
- runners may crash or misbehave
- logs and artifacts may be hostile

As a result:
- execution is isolated
- results are validated
- leases fence authority
- failures are expected

Security is enforced structurally, not heuristically.

---

## Principle 5: Failure Is Normal

Failures are not exceptional — they are expected.

The system must:
- tolerate crashes
- recover from partial execution
- handle duplicate events
- continue operating under degraded conditions

Design choices favor:
- explicit retries
- timeouts
- idempotent operations

Manual intervention should be rare.

---

## Principle 6: Human-in-the-Loop by Default

Automation must never remove human agency.

Delta CI may:
- explain failures
- suggest fixes
- generate candidate patches

Delta CI must not:
- apply code changes automatically
- deploy code
- bypass approvals
- override security policies

Humans remain responsible for final decisions.

---

## Principle 7: Convention Over Configuration

Delta CI prefers strong defaults.

- projects should work with minimal setup
- auto-detection is the primary path
- explicit configuration is an override, not a requirement

Configuration exists to:
- clarify intent
- codify non-standard behavior
- reduce ambiguity

YAML is not the product.

---

## Principle 8: Explainability Over Cleverness

Every decision should be explainable.

For any run, the system must be able to answer:
- why was this job run?
- why was this job skipped?
- why did this job fail?
- why was this retried?

Opaque behavior is considered a bug.

---

## Principle 9: Bounded Autonomy

Delta CI automates within **explicit boundaries**.

Boundaries include:
- time limits
- retry limits
- execution scopes
- AI capabilities

When in doubt, the system must:
- stop
- report
- wait for input

Unbounded automation is explicitly rejected.

---

## Principle 10: Design for Change

Delta CI is expected to evolve.

Design must allow:
- new planners
- new runner types
- new execution environments
- new AI capabilities

This is achieved through:
- explicit contracts
- versioned protocols
- loose coupling

Breaking changes must be documented via ADRs.

---

## Anti-Principles

Delta CI explicitly avoids:

- hidden magic
- implicit state mutation
- global mutable configuration
- long-lived workers with memory
- auto-deployments without approval
- “AI knows best” behavior

If something feels magical, it is probably wrong.

---

## Decision Escalation

When a design question arises:

1. Check this document
2. Check existing ADRs
3. If unresolved, create a new ADR

Design discussions without documentation do not scale.

---

## Related Documents

- architecture/overview.md
- architecture/components.md
- architecture/runner-protocol.md
- architecture/state-machines.md
- architecture/security-model.md
- design/diff-aware-planning.md
- adr/*

---

## Summary

Delta CI is built on conservative, explicit, and explainable design principles.

The system prioritizes:
- correctness over convenience
- safety over autonomy
- clarity over cleverness

These principles are non-negotiable and define the identity of the project.