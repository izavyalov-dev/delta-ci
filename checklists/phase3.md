# Phase 3 Implementation Checklist

This document defines the **Phase 3 implementation plan** for Delta CI.

Phase 3 is about **AI-assisted understanding**: faster, clearer explanations of
failures without sacrificing determinism, safety, or human control.

---

## Phase 3 Goals

By the end of Phase 3, Delta CI must:

- provide structured failure classification with confidence levels
- deliver AI-assisted explanations that are advisory and sanitized
- support human-in-the-loop fix suggestions with validation jobs
- keep AI optional, auditable, and fail-safe

Correctness, explainability, and safety remain mandatory.

---

## Guiding Rules

- AI is advisory only and never authoritative
- sanitize all inputs before AI (logs, artifacts, snippets)
- AI failures must not block CI execution
- prompts must be injection-resistant and size-bounded
- all AI outputs must be labeled, logged, and auditable
- any behavior change must update docs (and ADRs if needed)

---

## Milestone 3.1 — Structured Failure Classification v2

- [x] Expand rule-based classification signals (exit codes, timings, retries)
- [x] Include cache events and artifact metadata as signals
- [x] Persist classification signals and confidence per attempt
- [x] Expose classification in APIs and GitHub summaries
- [x] Add tests for deterministic classification paths

Success criteria:
- identical inputs always yield identical classifications
- confidence reflects uncertainty (no false precision)
- classification works without AI

---

## Milestone 3.2 — AI Explanation Pipeline

- [ ] Define provider-agnostic AI client interface and config
- [ ] Build sanitized input bundles (redacted, truncated, bounded)
- [ ] Add prompt templates with injection mitigation
- [ ] Enforce timeouts, circuit breakers, and graceful fallback
- [ ] Persist AI explanations as advisory artifacts with metadata

Success criteria:
- AI outage never blocks runs
- AI inputs are sanitized and size-bounded
- AI outputs are labeled and auditable

---

## Milestone 3.3 — Fix Suggestions + Validation Jobs

- [ ] Define patch format (unified diff) and storage for suggestions
- [ ] Create validation job type that applies patch in a sandbox
- [ ] Ensure validation runs are isolated and non-privileged
- [ ] Report validation results without modifying repo state
- [ ] Require explicit user approval to apply any fix

Success criteria:
- no auto-apply of AI fixes
- failed validation does not alter state
- users can inspect patch + validation outcome

---

## Milestone 3.4 — Human-in-the-Loop UX + Audit

- [ ] Surface AI explanations and confidence in APIs
- [ ] Add GitHub summaries with advisory labels and evidence links
- [ ] Provide policy toggles (enable/disable AI usage)
- [ ] Log all AI interactions with run/job correlation IDs
- [ ] Document new behaviors and update security notes

Success criteria:
- users can distinguish AI vs rule-based output
- audit trail exists for every AI interaction
- AI can be disabled without regressions

---

## Explicitly Out of Scope (Phase 3)

- auto-applied fixes or auto-merge
- AI-driven state transitions or retries
- unbounded log ingestion by AI
- provider-specific behavior in core logic

---

## Phase 3 Exit Criteria

Phase 3 is complete when:

- failure explanations are structured, labeled, and user-facing
- AI assistance is safe, optional, and audited
- validation jobs enforce human-in-the-loop fixes
- documentation matches behavior

If behavior and docs diverge, Phase 3 is not complete.

---

## Summary

Phase 3 focuses on **clarity without authority**:
explanations that help humans act, while the system remains deterministic and safe.
