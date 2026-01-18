# Phase 2 Implementation Checklist

This document defines the **Phase 2 implementation plan** for Delta CI.

Phase 2 is about **showing diff-aware value**: fewer jobs for common changes
without correctness regressions.

---

## Phase 2 Goals

By the end of Phase 2, Delta CI must:

- select jobs based on impact analysis with conservative fallback
- support basic monorepo structures and project boundaries
- persist and reuse validated recipes
- integrate conservative caching with explicit keys
- provide improved explainability for "why this ran"

Correctness, explainability, and safety remain mandatory.

---

## Guiding Rules

- deterministic and explainable planning
- expand the plan on uncertainty, never shrink it
- control plane owns state; runners execute only
- AI is advisory only and never authoritative
- untrusted logs/artifacts must be sanitized before analysis
- behavior changes require documentation updates (and ADRs if needed)

---

## Milestone 2.1 — Impact Analysis v1

- [ ] Define project ownership mapping from discovery inputs
- [ ] Compute impacted projects from diff paths and file types
- [ ] Propagate changes through known dependencies
- [ ] Treat unknown ownership or dependency gaps as global impact
- [ ] Emit per-job explanations tied to diff inputs

Success criteria:
- same inputs always produce the same plan
- unknowns trigger conservative expansion
- each job has a concrete "why this ran" reason

---

## Milestone 2.2 — Monorepo Support (Basic)

- [ ] Detect workspace roots and project boundaries
- [ ] Build per-project job graphs and aggregate into a run plan
- [ ] Handle shared config changes (e.g., tooling, ci.ai.yaml) as global impact
- [ ] Preserve allow-failure vs required classification per project
- [ ] Ensure explainability names affected projects explicitly

Success criteria:
- scoped changes run only affected project jobs
- global changes run all required jobs

---

## Milestone 2.3 — Recipe Persistence and Reuse

- [ ] Define recipe schema (jobs, order, tools, caches, artifacts)
- [ ] Define repository fingerprint inputs and hashing
- [ ] Persist recipes immutably with fingerprint and timestamps
- [ ] Implement selection order: config > recipe match > discovery > fallback
- [ ] Expose recipe usage and selection rationale in plan output

Success criteria:
- stable repos reuse recipes deterministically
- fingerprint changes trigger re-discovery

---

## Milestone 2.4 — Conservative Cache Integration

- [ ] Define cache config per job in the control plane
- [ ] Implement dependency and toolchain cache mounts in runners
- [ ] Enforce fork PR read-only or no-cache policy
- [ ] Ensure cache key inputs are deterministic and secret-free
- [ ] Report cache hit/miss and keys in job metadata

Success criteria:
- cache failures never block job correctness
- cache behavior is transparent and explainable

---

## Milestone 2.5 — Explainability + Validation

- [ ] Include "why this ran" reasons in API and GitHub summaries
- [ ] Surface skipped jobs and explicit fallback reasons
- [ ] Dogfood on the Delta CI repo with impact-based reductions
- [ ] Add/refresh documentation for all new behaviors

Success criteria:
- reduced job counts for common changes without regressions
- explainability is complete and user-facing

---

## Explicitly Out of Scope (Phase 2)

- multi-runner scheduling and orchestration
- advanced build caching beyond explicit opt-in
- AI-generated fixes or auto-applied patches
- enterprise multi-tenant features or SaaS concerns
- Kubernetes-first deployment or large-scale tuning

---

## Phase 2 Exit Criteria

Phase 2 is complete when:

- impact-based planning reduces unnecessary jobs safely
- monorepo boundaries are honored in planning
- recipes are persisted, selected, and explained
- caching is conservative, deterministic, and fork-safe
- documentation and behavior are aligned

If behavior and docs diverge, Phase 2 is not complete.

---

## Summary

Phase 2 is about **proving diff-aware value**:
fewer jobs, faster feedback, and the same correctness guarantees.
