# Roadmap

This document outlines the **planned evolution of Delta CI**.

The roadmap is a statement of intent, not a promise.  
Priorities may change based on real-world usage, feedback, and constraints.

Delta CI favors **shipping small, correct pieces** over large, speculative features.

---

## Roadmap Principles

The roadmap follows these rules:

- correctness before optimization
- architecture before features
- dogfooding before expansion
- explicit scope over feature creep
- stability before performance tuning

Features are added only when they reinforce core principles.

---

## Phase 0: Foundation (Current)

**Goal:** Establish a correct, documented, and testable core.

### Completed / In Progress
- Core architecture definition
- Control plane / data plane separation
- Runner lease protocol
- Explicit state machines
- Security and trust model
- Diff-aware planning design
- Documentation-first approach

### Deliverables
- Working control plane skeleton
- Minimal runner implementation
- End-to-end job execution (single repo)
- Self-hosted local deployment

---

## Phase 1: Minimal Usable CI

**Goal:** Delta CI can replace a basic CI setup for its own repository (dogfooding).

### Scope
- GitHub integration (checks + PR comments)
- Single-runner execution model
- Basic diff-aware planning
- Artifact and log storage
- Manual reruns and cancellation
- Basic failure explanations (non-AI or limited AI)

### Explicit Non-Goals
- Enterprise features
- Advanced caching
- Multi-tenant SaaS concerns

---

## Phase 2: Diff-Aware Value

**Goal:** Demonstrate clear advantages over traditional CI.

### Scope
- Impact-based job selection
- Monorepo support (basic)
- Recipe persistence and reuse
- Conservative cache integration
- Improved explainability (“why this ran”)

### Validation Criteria
- Fewer jobs executed for common changes
- Faster feedback for developers
- No regression in correctness

---

## Phase 3: AI-Assisted Understanding

**Goal:** Reduce time-to-understanding for failures.

### Scope
- AI-assisted failure explanation
- Structured failure classification
- Human-in-the-loop fix suggestions
- Validation jobs for AI-generated patches

### Constraints
- AI remains advisory only
- All fixes require explicit user approval
- AI outages must not affect CI correctness

---

## Phase 4: Scalability and Hardening

**Goal:** Operate reliably under sustained load.

### Scope
- Horizontal runner scaling
- Backpressure and queue tuning
- Improved observability
- Performance profiling
- Stress and chaos testing

### Non-Goals
- Global multi-region support
- Aggressive performance tuning without evidence

---

## Phase 5: Extensibility

**Goal:** Enable ecosystem growth without architectural drift.

### Scope
- Plugin points for planners and analyzers
- Multiple runner types (VM, container, remote)
- Additional VCS providers
- External integrations (notifications, dashboards)

### Requirements
- Stable contracts
- Versioned protocols
- Backward compatibility

---

## Out of Scope (Explicitly)

The following are **not planned**:

- Autonomous deployments
- Auto-merge decisions
- Replacing build systems
- IDE integrations (short term)
- Proprietary AI lock-in

These may be revisited only with strong justification.

---

## Milestone Definition

A milestone is considered complete when:

- behavior is documented
- state machines are updated
- failure modes are understood
- rollback or disable paths exist

Shipping undocumented behavior is considered a failure.

---

## Contribution Alignment

Contributions are welcome when they:

- align with the current phase
- reinforce existing principles
- do not introduce hidden coupling
- include documentation updates

Large features without documentation will be rejected.

---

## Re-Evaluation Policy

The roadmap should be re-evaluated:
- after major dogfooding milestones
- after significant user feedback
- after architectural changes

Roadmap changes must be documented.

---

## Summary

Delta CI’s roadmap prioritizes:

1. Correctness
2. Explainability
3. Safety
4. Incremental value

Growth is intentional, not reactive.  
The roadmap exists to keep the project focused and sustainable.