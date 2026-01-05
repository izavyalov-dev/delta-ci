# Delta CI Agent Guide

This guide summarizes how to work on this repo as an AI or human assistant. It is a quick reference; full context lives in `docs/`.

## Orientation
- Start with `docs/README.md`, then architecture files in `docs/architecture/` and design docs in `docs/design/`.
- ADRs in `docs/adr/` are authoritative; do not contradict them without a new ADR.
- If behavior is not documented, treat it as undefined and avoid implementing it.

## Core Principles (keep these top of mind)
- Diff-aware by default: plans are driven by *what changed*; “run everything” is a conservative fallback (`docs/design/diff-aware-planning.md`).
- Deterministic, explainable control plane owns state; data plane only executes untrusted jobs (`docs/architecture/control-plane.md`, `docs/architecture/data-plane.md`).
- Lease-based execution fences authority; stale leases must be rejected (`docs/architecture/runner-protocol.md`, `docs/architecture/state-machines.md`, ADR-0005).
- Security first: runners, logs, and artifacts are untrusted; secrets never flow to fork PRs; treat `lease_id` as a secret (`docs/architecture/security-model.md`).
- AI is advisory only—never authoritative, never applies patches automatically, and must use sanitized inputs (`docs/design/ai-usage.md`).
- Human-in-the-loop is mandatory; automation stays within explicit, time-bounded limits.

## Architecture Snapshot
- Control plane components: API Gateway/BFF, Orchestrator (state authority), Planner (decides what to run), Failure Analyzer (explains), Status Reporter (talks to VCS). None execute user code.
- Data plane: runner controller + ephemeral single-job runners with restricted network and isolated filesystems. Runners communicate only via lease/heartbeat/complete/cancel messages.
- State machines for runs/jobs/leases are the source of truth for allowed transitions (`docs/architecture/state-machines.md`); illegal transitions are bugs.

## Technology and Ops Defaults
- Stack: Go services and runner, PostgreSQL for state, at-least-once queue (Postgres-backed initially), S3-compatible artifact storage, HTTP+JSON APIs, Prometheus/structured logs, optional OpenTelemetry tracing (ADR-0006).
- Deployment expectations: self-host friendly; start with simple/local setups before Kubernetes (`docs/operations/deployment.md`). Observability and auditability are required (`docs/operations/observability.md`).
- Caching is explicit and conservative; cache keys must be deterministic and never include secrets (`docs/design/caching-strategy.md`).

## Working Guidance
- Follow Phase 0 checklist in `checklists/phase0.md` when extending the early implementation: correctness over speed; no shortcuts around leases/state machines; one-runner/single-job paths are acceptable.
- Keep plans explainable: be able to state why each job runs or is skipped. On ambiguity, expand the plan rather than shrinking it.
- Treat artifacts/logs as hostile input; sanitize before analysis or AI use. Never log secrets or lease IDs.
- Any change that alters behavior must include documentation updates; new design shifts require ADRs.
- Prefer small, deterministic increments; retries, cancellations, and timeouts are orchestrator-driven only.

## Quick Pointers
- Terminology reference: `docs/reference/terminology.md`
- Protocol schemas: `docs/reference/runner-messages.md`
- External/internal API contracts: `docs/reference/api-contracts.md`
- Configuration contract: `docs/reference/configuration.md`
- Roadmap and scope: `docs/roadmap.md`, ADR-0001/0002 for intent and non-goals
