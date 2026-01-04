# ADR-0002: Why Diff-Aware CI

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Traditional CI systems are built around **static pipelines**:
- a predefined set of steps
- executed fully on every change
- regardless of what actually changed

This model was acceptable when:
- repositories were small
- build times were short
- compute was cheap
- feedback speed was not critical

In modern systems, this model causes significant problems:
- monorepos trigger massive pipelines for trivial changes
- developers wait for irrelevant jobs
- CI cost scales linearly with repository size
- signal-to-noise ratio degrades

Most CI systems treat **change awareness** as an optimization (path filters, conditional steps), not as a first-class design principle.

---

## Decision

Delta CI adopts **diff-aware planning** as a core architectural concept.

This means:
- execution plans are derived from *what changed*
- static pipelines are a fallback, not a default
- “run everything” is a conservative escape hatch, not the baseline

Diff-aware planning is not an optimization layer; it is the primary execution model.

---

## Rationale

### 1. Work Should Be Proportional to Change

A one-line documentation change should not:
- rebuild the entire system
- run all integration tests
- consume minutes of compute

Diff-aware CI aligns cost and feedback with impact.

---

### 2. Static Pipelines Encode Ignorance

Static pipelines assume:
- every change is equally risky
- all jobs are always relevant

This is rarely true and leads to:
- wasted execution
- slower feedback
- reduced developer trust in CI

Diff-aware planning encodes *knowledge* about the system.

---

### 3. Manual Optimization Does Not Scale

Existing approaches require:
- hand-maintained path filters
- brittle conditional logic
- tribal knowledge encoded in YAML

These approaches:
- rot over time
- are hard to review
- break silently

Delta CI treats planning as a **first-class problem**, not as a YAML trick.

---

### 4. Explainability Becomes Possible

When the system reasons about changes explicitly, it can answer:
- why a job ran
- why a job was skipped
- which files triggered execution

This is impossible to do reliably with opaque pipelines.

Explainability is a direct consequence of diff-aware design.

---

### 5. AI Assistance Becomes Safer

AI can assist with:
- impact analysis
- discovery
- explanation

But only if:
- the system already understands what changed
- decisions are bounded and explainable

Diff-aware planning provides the structure that makes safe AI usage possible.

---

## Consequences

### Positive

- reduced CI execution time
- lower compute cost
- faster developer feedback
- improved signal quality
- better explainability
- natural fit for monorepos

---

### Negative

- higher implementation complexity
- need for robust fallbacks
- conservative behavior in ambiguous cases
- more upfront design work

These costs are accepted to achieve long-term benefits.

---

## Fallback and Safety

Diff-aware planning is **conservative by design**.

Rules:
- unknown impact → run more, not less
- ambiguity → expand the plan
- failure to analyze → fail explicitly or fall back

Correctness is never sacrificed for optimization.

---

## Alternatives Considered

### Path Filters in Static Pipelines
Rejected because:
- manual and brittle
- hard to maintain
- lack explainability

---

### Always Run Everything
Rejected because:
- wasteful at scale
- slow feedback loops
- poor developer experience

---

### AI-Decided Execution Without Guards
Rejected because:
- nondeterministic
- unsafe for CI
- not auditable

---

## Relationship to Other Decisions

- Enables: ADR-0005 (Runner Lease Model)
- Depends on: ADR-0004 (Control vs Data Plane)
- Informs: Design Principles, Diff-Aware Planning

---

## Summary

Diff-aware CI is the core differentiator of Delta CI.

By making **change awareness** the primary driver of execution, Delta CI:
- reduces waste
- improves feedback
- enables explainability
- provides a safe foundation for AI assistance

This decision defines the identity of the project.