# Failure Analysis

This document describes how **Delta CI analyzes failures**, classifies their causes, and assists developers in resolving them.

Failure analysis exists to reduce time-to-understanding and time-to-fix — without hiding complexity or making unsafe assumptions.

Failure analysis is **diagnostic**, not corrective.

---

## Goals

Failure analysis aims to:

- explain *why* a job failed
- distinguish user errors from infrastructure issues
- reduce log-diving and guesswork
- assist, but not replace, human decision-making

Failure analysis must remain safe, explainable, and auditable.

---

## Scope

Failure analysis applies to:
- job execution failures
- job timeouts
- canceled executions (partial analysis)
- flaky or retryable failures

It does not:
- decide retries
- change job state
- apply fixes automatically
- override orchestrator decisions

---

## Inputs

Failure analysis operates on **post-execution data** only.

### Allowed Inputs
- exit codes
- job metadata (language, toolchain, job type)
- sanitized and truncated logs
- structured test reports (e.g., JUnit/TRX)
- artifact metadata
- execution timings

### Forbidden Inputs
- secrets
- raw environment dumps
- private tokens or credentials
- unbounded logs or binaries

All inputs are treated as **untrusted**.

---

## Failure Classification

Failures are classified into **high-level categories**.

### 1. User Failures
Errors caused by the code under test.

Examples:
- compilation errors
- failing unit tests
- lint/style violations
- missing dependencies declared in code

Typical properties:
- deterministic
- reproducible
- non-retryable

---

### 2. Infrastructure Failures
Errors caused by the execution environment.

Examples:
- network timeouts
- registry unavailability
- runner startup failures
- resource exhaustion

Typical properties:
- transient
- retryable
- non-deterministic

---

### 3. Tooling Failures
Errors caused by tools or configuration mismatches.

Examples:
- incompatible SDK versions
- corrupted caches
- misconfigured build scripts

May be retryable or non-retryable depending on context.

---

### 4. Flaky Failures
Failures that appear non-deterministic but are not clearly infra-related.

Examples:
- race conditions in tests
- timing-sensitive assertions
- order-dependent failures

Flakiness detection is heuristic and advisory.

---

## Classification Strategy

Classification follows a layered approach:

1. **Rule-based detection**
   - exit code patterns
   - known error signatures
   - tool-specific failure modes

2. **Contextual signals**
   - previous run outcomes
   - retry success/failure
   - timing anomalies

3. **AI-assisted interpretation**
   - only for explanation
   - never authoritative

Classification confidence must be surfaced to the user.

---

## Phase 1 Rule Set

Phase 1 uses a minimal, deterministic rule set based on:
- job name
- exit code
- sanitized runner summary
- artifact metadata (URIs only)

Rules (in order):
- timeouts, OOM, disk full, killed signals → **INFRA** (medium/high confidence)
- network/connection errors → **INFRA** (high confidence)
- command not found / missing executable / permission denied → **TOOLING** (medium/high confidence)
- job name includes `lint`/`vet` → **USER** (medium confidence)
- job name includes `test` → **USER** (medium confidence)
- job name includes `build` → **USER** (medium confidence)
- otherwise → **USER** (low confidence)

Logs are never fetched for analysis in Phase 1. Only artifact URIs are referenced.

---

## Explanation Generation

Failure explanations must:

- be concise
- reference observable evidence
- avoid speculation
- distinguish facts from suggestions

A good explanation answers:
- what failed
- where it failed
- why it failed (likely cause)
- what to check next

---

## Partial Failures and Cancellation

For canceled or interrupted jobs:
- analysis may be incomplete
- explanations must reflect uncertainty
- partial logs should be referenced explicitly

The system must not invent causes when data is incomplete.

---

## Fix Suggestions

Fix suggestions are **optional and advisory**.

### Allowed Suggestions
- formatting fixes
- dependency updates
- test snapshot updates
- configuration corrections

### Constraints
- suggestions must be clearly labeled
- suggestions must be validated via a sandbox job
- suggestions must not be applied automatically

Users always retain final control.

---

## Validation Loop

When a fix is proposed:

1. Generate candidate patch
2. Create a validation job
3. Apply patch in isolation
4. Run relevant checks
5. Report outcome
6. Await user decision

A failed validation must not modify repository state.

---

## Explainability and Transparency

For every analyzed failure, Delta CI must be able to explain:

- how the failure was classified
- which signals were used
- whether AI was involved
- what uncertainty remains

Opaque analysis is considered a bug.

---

## Logging and Audit

Failure analysis events must be:
- logged
- correlated with run/job IDs
- traceable to inputs and outputs

Audit logs must not contain sensitive data.

---

## Failure Analysis Failures

If failure analysis itself fails:
- CI execution must continue
- a fallback explanation must be shown
- the error must be logged

Failure analysis must never block CI completion.

---

## Security Considerations

- all inputs are sanitized
- AI interactions are constrained
- logs and artifacts are untrusted
- patches are validated in isolation

Security boundaries must be respected at all times.

---

## Related Documents

- design/principles.md
- design/ai-usage.md
- architecture/security-model.md
- architecture/runner-protocol.md

---

## Summary

Failure analysis in Delta CI exists to assist understanding, not to automate judgment.

By combining rule-based classification, careful AI assistance, and strict validation boundaries, Delta CI provides meaningful feedback while preserving safety, correctness, and human control.
