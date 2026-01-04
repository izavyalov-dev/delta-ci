# Security Model

This document describes the **security model and trust boundaries** of Delta CI.

Delta CI executes untrusted code by design.  
Security is therefore a **first-class architectural concern**, not an add-on.

This document defines:
- trust boundaries
- threat assumptions
- secret handling rules
- runner isolation guarantees
- AI safety constraints

---

## Security Goals

Delta CI is designed to:

- safely execute **untrusted user code**
- prevent secret leakage
- limit blast radius of compromised runners
- provide auditable and deterministic behavior
- remain secure under partial system compromise

Security decisions favor **safety over convenience**.

---

## Threat Model

### Assumed Threats

Delta CI assumes the following are possible:

- malicious code in pull requests
- forked repositories controlled by attackers
- runners crashing, stalling, or behaving maliciously
- network interception or partial outages
- poisoned logs or artifacts
- prompt injection via logs, errors, or source code

### Non-Goals

Delta CI does NOT attempt to:
- protect against compromised host kernels
- defend against physical access attacks
- sandbox arbitrary zero-day kernel exploits

The system relies on standard container / VM isolation primitives.

---

## Trust Boundaries

### Control Plane (Trusted)
Components considered trusted:
- Orchestrator
- Planner
- Failure Analyzer
- Status Reporter
- Database
- Secrets Manager

The control plane:
- never executes user code
- never mounts user-provided filesystems
- never trusts runner-provided data blindly

---

### Data Plane (Untrusted)
Components considered untrusted:
- Runners
- Runner images
- Job logs and artifacts
- Source code under test

Runners are treated as potentially hostile after job start.

---

## Runner Isolation Model

### Execution Environment

Runners must be:
- ephemeral
- single-job only
- isolated at OS/process level

Recommended isolation:
- containers with seccomp/apparmor profiles, or
- lightweight VMs (e.g., Firecracker)

### Filesystem Isolation

- each runner gets a fresh filesystem
- no shared writable volumes between runners
- caches must be scoped and sanitized

### Network Isolation

By default:
- outbound network access is restricted
- inbound access is denied
- only explicitly allowed endpoints are reachable (e.g., package registries)

Network access policies must be explicit and auditable.

---

## Secrets Handling

### Core Rules

- secrets are **never persisted** by runners
- secrets are **never written to logs**
- secrets are **never available to fork PRs**
- secrets are short-lived and scoped

### Secret Distribution

Secrets are provided:
- just-in-time
- per job
- via environment variables or mounted files
- using short-lived credentials (OIDC preferred)

### Fork PR Policy

For forked pull requests:
- no secrets
- read-only repository access
- no deployment credentials
- no cloud provider access

This policy is non-overridable.

---

## Artifact and Log Safety

Artifacts and logs are considered **untrusted input**.

Rules:
- logs must be redacted before AI processing
- artifacts must not be executed by the control plane
- parsers must treat all content as hostile

Artifact viewers must:
- sanitize HTML
- escape control characters
- avoid inline execution

---

## AI Safety Boundaries

### Allowed AI Inputs

AI systems may receive:
- truncated logs
- structured error summaries
- metadata (exit codes, step names)
- sanitized code snippets (size-limited)

### Forbidden AI Inputs

AI systems must never receive:
- secrets
- raw environment variables
- private keys
- access tokens
- full repository history by default

### AI Output Constraints

AI may:
- explain failures
- suggest fixes
- generate candidate patches

AI must NOT:
- apply patches automatically
- deploy code
- modify infrastructure
- bypass policy checks

Human approval is mandatory for all mutations.

---

## Lease and Protocol Security

- `lease_id` is a capability token
- possession of `lease_id` grants execution rights
- `lease_id` must be treated as secret

Rules:
- never log `lease_id`
- never expose `lease_id` to user code
- reject all protocol messages with stale or invalid leases

---

## Cancellation Safety

Cancellation exists to:
- stop runaway jobs
- limit damage
- reclaim resources

Rules:
- cancellation must not skip artifact upload when possible
- forced cancellation must still preserve logs
- late completions from canceled leases must be rejected

---

## Observability and Audit

Security-relevant events must be logged:
- lease grants and expirations
- cancellations and forced cancels
- secret access
- failed authentication
- policy violations

Audit logs must be:
- immutable
- timestamped
- correlated with run/job IDs

---

## Failure Containment

If a runner is compromised:
- it cannot affect other jobs
- it cannot mutate orchestrator state
- it cannot finalize jobs without a valid lease
- it cannot escalate privileges via secrets

The worst-case impact is limited to:
- its own job execution
- its own artifacts/logs

---

## Security Review Expectations

Security-sensitive changes must:
- update this document if assumptions change
- include explicit threat reasoning
- preserve or strengthen isolation guarantees

If a behavior is security-relevant and undocumented, it must be treated as a bug.

---

## Related Documents

- overview.md
- components.md
- runner-protocol.md
- state-machines.md
- design/ai-usage.md
- ADR-0004-control-vs-data-plane.md
- ADR-0005-runner-lease-model.md

---

## Summary

Delta CI assumes hostile input and unreliable execution.

Security is enforced through:
- strict trust boundaries
- ephemeral execution
- lease-based fencing
- explicit cancellation and timeout rules
- constrained and audited AI usage

This model prioritizes containment, predictability, and auditability over convenience.