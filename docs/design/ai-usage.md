# AI Usage

This document defines **where and how AI is allowed to operate** within Delta CI.

AI is a powerful tool, but also a potential source of nondeterminism and security risk.  
Delta CI treats AI as an **assistant**, never as an authority.

If AI behavior is not explicitly allowed in this document, it is forbidden.

---

## Goals

AI usage in Delta CI aims to:

- improve developer feedback quality
- reduce time-to-understanding for failures
- assist with discovery and planning
- remain safe, explainable, and auditable

AI must never compromise correctness, security, or human control.

---

## Core Principles

### AI Is Advisory, Not Authoritative

AI may:
- suggest
- explain
- summarize
- rank options

AI must not:
- enforce decisions
- mutate state
- bypass rules
- act autonomously

All final decisions are made by deterministic system components.

---

### Determinism First

- Core behavior must be deterministic without AI.
- AI output must not affect correctness.
- AI failure must degrade gracefully.

If AI is unavailable, the system must continue operating correctly.

---

## Allowed AI Capabilities

### 1. Failure Explanation

AI may:
- analyze sanitized logs
- classify likely failure causes
- produce human-readable explanations
- suggest next debugging steps

Constraints:
- logs must be redacted and truncated
- outputs must be clearly marked as advisory
- explanations must reference observable evidence

---

### 2. Fix Suggestions (Human-in-the-Loop)

AI may:
- propose candidate patches
- suggest configuration changes
- recommend test updates

Rules:
- fixes must never be applied automatically
- fixes must be validated in a sandboxed job
- users must explicitly accept any change

AI-generated patches are treated as untrusted input.

---

### 3. Discovery Assistance

AI may:
- interpret repository documentation
- rank candidate build/test commands
- suggest project boundaries in monorepos

AI must not:
- execute commands
- persist recipes
- override validation failures

All AI suggestions must be validated through execution.

---

### 4. Plan Explanation

AI may:
- explain why jobs were selected
- explain why jobs were skipped
- summarize planning decisions

AI may not:
- alter the execution plan
- remove required jobs
- reduce safety checks

---

## Forbidden AI Capabilities

AI must never:

- apply code changes automatically
- deploy or release software
- modify infrastructure
- access raw secrets
- access full environment variables
- receive full repository history by default
- make retry or cancellation decisions
- override explicit configuration
- bypass security policies

Any violation is a critical security issue.

---

## Input Constraints

### Sanitization

Before sending data to AI:
- secrets must be redacted
- tokens and credentials removed
- logs truncated to bounded size
- binary data rejected

Sanitization is mandatory and non-optional.

---

### Allowed Inputs

AI may receive:
- structured error summaries
- exit codes
- step names
- truncated logs
- small, relevant code snippets
- metadata (job name, language, toolchain)

---

### Forbidden Inputs

AI must never receive:
- secrets
- private keys
- access tokens
- raw environment dumps
- unbounded logs
- private customer data

---

## Output Constraints

AI output must be:

- clearly labeled as AI-generated
- treated as advisory
- logged for auditability
- size-bounded

AI output must not:
- contain executable instructions without validation
- reference hidden system state
- include hallucinated guarantees

---

## Validation of AI Output

When AI proposes a fix:

1. The fix is generated as a patch
2. A validation job is created
3. The patch is applied in a sandbox
4. Relevant tests are executed
5. Results are reported
6. User decides whether to apply the fix

Validation failure must never modify state.

---

## Prompt Injection Mitigation

Delta CI assumes logs and source code may attempt to manipulate AI.

Mitigations include:
- strict prompt templates
- separating system instructions from user content
- rejecting instruction-like content from logs
- minimizing context size

AI is never allowed to follow instructions originating from logs or code.

---

## Provider Abstraction

AI usage must be provider-agnostic.

Requirements:
- pluggable AI backends
- consistent input/output contracts
- timeouts and circuit breakers
- graceful degradation

No provider-specific behavior may leak into core logic.

---

## Auditing and Observability

All AI interactions must be:
- logged (metadata only, no raw sensitive inputs)
- traceable to run/job IDs
- bounded in cost and time

AI usage must be observable and accountable.

---

## Failure Handling

If AI fails:
- the system continues without AI
- fallback explanations are provided
- planning and execution remain unaffected

AI failure must never block CI execution.

---

## Related Documents

- design/principles.md
- design/diff-aware-planning.md
- design/failure-analysis.md
- architecture/security-model.md
- adr/ADR-0002-why-diff-aware-ci.md

---

## Summary

AI in Delta CI is deliberately constrained.

It exists to:
- help humans understand failures
- assist with discovery
- improve clarity and feedback

It does not replace deterministic logic, human judgment, or explicit configuration.

AI is a tool — not a decision-maker.