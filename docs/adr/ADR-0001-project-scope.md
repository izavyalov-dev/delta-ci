# ADR-0001: Project Scope and Goals

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Continuous Integration systems are a critical part of modern software delivery, but most existing CI solutions are built around **static pipelines** and **run-all-by-default** execution models.

These systems:
- waste compute on unchanged code paths
- provide noisy feedback
- require significant upfront configuration
- treat failures as logs to read, not problems to understand

At the same time, recent advances in AI make it possible to improve **understanding and explainability**, but naive application of AI introduces risks around nondeterminism, security, and loss of human control.

Delta CI was initiated to explore a different approach.

---

## Decision

Delta CI is defined as:

> An **open-source, AI-native, diff-aware CI system** that prioritizes correctness, explainability, and human-in-the-loop automation.

The project will focus on:
- deciding *what needs to run* based on changes
- executing jobs safely and deterministically
- explaining failures clearly
- assisting humans without replacing them

The project will **not** attempt to solve every CI problem.

---

## Goals

Delta CI explicitly aims to:

1. **Reduce unnecessary execution**
   - use diff-aware planning
   - avoid “run everything” by default

2. **Improve signal quality**
   - clear explanations for why jobs ran or failed
   - distinguish user errors from infrastructure failures

3. **Remain safe by design**
   - strict separation of control plane and data plane
   - untrusted execution model
   - explicit security boundaries

4. **Preserve human control**
   - AI is advisory only
   - no autonomous deployments or merges
   - explicit approvals for all mutations

5. **Be viable as open source**
   - permissive license
   - self-host friendly
   - readable and auditable design

---

## Non-Goals

Delta CI explicitly does **not** aim to:

- replace build systems (e.g. Bazel, Gradle, Make)
- become a full CD or deployment platform
- provide autonomous remediation or self-healing
- hide complexity behind opaque automation
- optimize for every language or ecosystem initially
- compete on feature parity with mature CI platforms

These non-goals are intentional and enforced.

---

## Constraints

The following constraints apply to all future design decisions:

- correctness over performance
- determinism over cleverness
- safety over autonomy
- explicit contracts over implicit behavior
- documentation-first development

Violating these constraints requires a new ADR.

---

## Consequences

### Positive

- Clear project identity and boundaries
- Easier architectural reasoning
- Strong foundation for contributor alignment
- Reduced risk of feature creep
- Enterprise-acceptable security posture

### Negative

- Slower feature expansion
- Fewer “wow” automation features initially
- Higher upfront design and documentation cost

These tradeoffs are accepted.

---

## Alternatives Considered

### “Traditional CI with AI bolted on”
Rejected because:
- keeps static pipeline model
- limits impact of diff-awareness
- risks AI-driven nondeterminism

### “Autonomous AI CI”
Rejected because:
- unsafe for untrusted code
- unacceptable loss of human control
- difficult to audit or reason about

### “Minimal CI runner only”
Rejected because:
- does not address planning or explainability
- offers little differentiation

---

## Related Decisions

- ADR-0002: Why Diff-Aware CI
- ADR-0003: License Choice
- ADR-0004: Control Plane vs Data Plane
- ADR-0005: Runner Lease Model

---

## Summary

Delta CI exists to explore a **more intelligent but still conservative** model of CI.

By explicitly defining scope and non-goals early, the project protects itself from uncontrolled growth and architectural drift.

This ADR establishes the foundation against which all future decisions are evaluated.