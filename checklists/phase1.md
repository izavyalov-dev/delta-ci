# Phase 1 Implementation Checklist

This document defines the **Phase 1 implementation plan** for Delta CI.

Phase 1 is about **becoming minimally usable for real work**.
The goal is to dogfood Delta CI on its own repository with GitHub integration.

---

## Phase 1 Goals

By the end of Phase 1, Delta CI must:

- integrate with GitHub checks and PR comments
- support manual reruns and cancellations
- produce explainable plans based on diffs
- store and link logs/artifacts safely
- provide basic failure explanations
- dogfood on the Delta CI repo

Correctness, explainability, and safety remain mandatory.

---

## Guiding Rules

- deterministic and explainable planning
- conservative fallbacks on uncertainty
- control plane owns all state
- runners and artifacts are untrusted
- AI is advisory only (never authoritative)

---

## Milestone 1.1 — GitHub Integration (Checks + PR Comments)

☑ Implement webhook ingestion with idempotency  
☑ Normalize push and PR events into run creation  
☑ Implement Status Reporter for GitHub checks  
☑ Post PR comment summaries with run results  
☑ Ensure updates are idempotent and replay-safe  

Success criteria:
- duplicate webhooks do not create duplicate runs
- check status matches orchestrator state
- PR comments are updated or replaced, not spammed

---

## Milestone 1.2 — Run Management (Rerun + Cancel)

☑ Implement run cancel API  
☑ Implement run rerun API  
☑ Propagate cancel to active jobs  
☑ Enforce state machine transitions for cancel/rerun  
☑ Surface cancel/rerun in GitHub comments or checks  

Success criteria:
- cancel moves run to CANCEL_REQUESTED and finalizes correctly
- rerun creates a new run attempt without mutating history

---

## Milestone 1.3 — Planner v1 (Diff-Aware, Conservative)

☑ Implement discovery inputs (build files, README/CONTRIBUTING)  
☑ Implement minimal impact analysis from diff paths  
☑ Construct an explainable plan with required vs allow-failure jobs  
☑ Provide explicit “why this ran” reasoning  
☑ Fallback to “run everything” on uncertainty  

Success criteria:
- planner output is deterministic for the same inputs
- every job has a concrete explanation

---

## Milestone 1.4 — Artifact and Log Retrieval

☐ Store logs and artifacts in external storage  
☐ Persist artifact references in control plane  
☐ Expose APIs to fetch log and artifact URLs  
☐ Ensure logs are treated as untrusted input  

Success criteria:
- run/job APIs provide links to logs and artifacts
- failures in upload do not corrupt job state

---

## Milestone 1.5 — Failure Explanations (Rule-Based First)

☐ Implement rule-based failure classification  
☐ Generate a concise failure explanation summary  
☐ Ensure inputs are sanitized and size-bounded  
☐ Provide links to relevant logs and artifacts  
☐ Add AI hook points but keep them disabled by default  

Success criteria:
- explanations are based on observable data only
- AI output is clearly labeled and advisory if enabled

---

## Milestone 1.6 — Dogfooding on Delta CI

☐ Run Delta CI on its own repo via GitHub  
☐ Minimum jobs: build + test  
☐ Validate cancel and rerun workflows  
☐ Validate fallback planning behavior  

Success criteria:
- consistent, repeatable runs on PRs and main
- users can explain why jobs ran or were skipped

---

## Explicitly Out of Scope (Phase 1)

- multi-runner scheduling
- advanced caching or cross-run reuse
- AI-generated fixes or auto-applied patches
- multi-tenant SaaS features
- Kubernetes or large-scale deployment

---

## Phase 1 Exit Criteria

Phase 1 is complete when:

- GitHub checks and PR comments reflect orchestrator state
- rerun and cancel are stable and state-safe
- planner emits explainable, conservative plans
- logs and artifacts are stored and linked safely
- Delta CI can dogfood its own repository reliably

If behavior and docs diverge, Phase 1 is not complete.

---

## Summary

Phase 1 is about **utility with trust**.

Delta CI must be usable on real changes,
without sacrificing determinism or safety.
