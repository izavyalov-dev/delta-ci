# Operations Documentation

This directory contains **operational guidance** for running and maintaining **Delta CI**.

These documents are intended for:
- self-hosted operators
- early adopters
- contributors running Delta CI in development or staging
- anyone responsible for keeping Delta CI healthy in production

The focus is on **practical operability**, not vendor-specific tooling.

---

## What’s in This Directory
```
operations/
├─ README.md              # This file — entry point and runbook index
├─ deployment.md          # Deployment models and infrastructure assumptions
├─ scaling.md             # Scaling strategies and capacity planning
├─ observability.md       # Metrics, logs, traces, and alerting
├─ dogfooding.md          # Phase 0 dogfooding workflow
└─ disaster-recovery.md   # Failure scenarios and recovery procedures
```

---

## How to Read These Docs

### New Operators

If this is your first time running Delta CI:

1. **deployment.md**
   - choose a deployment model
   - understand required infrastructure
2. **observability.md**
   - know what “healthy” looks like
   - set up basic metrics and logs
3. **disaster-recovery.md**
   - understand failure modes *before* they happen

---

### Contributors and Developers

If you are running Delta CI locally or contributing:

- read **deployment.md** (local / single-node sections)
- skim **scaling.md** to understand architectural constraints
- use **observability.md** when debugging state transitions or leases

---

## Operational Assumptions

Delta CI is built on these operational assumptions:

- failure is normal
- runners are disposable
- state is authoritative only in the database
- queues and networks are at-least-once
- late messages are expected and must be safe
- leases, not workers, define execution authority

If an operational scenario contradicts these assumptions, it should be treated as a bug.

---

## Minimal Runbook Index

Use this as a quick reference during incidents.

### CI appears stuck / no progress
- Check queue depth and age (scaling.md)
- Check runner availability (scaling.md)
- Check lease expiration rate (observability.md)
- Check DB write latency (observability.md)

---

### Jobs retry unexpectedly
- Check lease TTL vs heartbeat interval (scaling.md)
- Look for runner crashes or network issues (observability.md)
- Verify orchestrator latency (observability.md)

---

### Many jobs re-run at once
- Inspect lease expiration spikes (observability.md)
- Check orchestrator restarts or DB stalls (disaster-recovery.md)

---

### Logs or artifacts missing
- Check artifact store availability (disaster-recovery.md)
- Verify runner upload permissions (deployment.md)
- Confirm retention policies (deployment.md)

---

### CI unavailable / API errors
- Check orchestrator health (observability.md)
- Check DB connectivity (disaster-recovery.md)
- Check queue health (disaster-recovery.md)

---

## Operational Anti-Patterns

Avoid:
- long-lived, stateful runners
- manual DB state edits
- disabling lease expiration
- widening runner network access to “fix” flakiness
- retrying failures without classification

These patterns undermine safety and correctness.

---

## When to Update These Docs

You should update the operations docs when:
- adding a new critical dependency
- changing lease or queue semantics
- introducing new failure modes
- altering security or isolation guarantees
- changing scaling behavior

Operational behavior must never drift undocumented.

---

## Related Documentation

- architecture/overview.md
- architecture/control-plane.md
- architecture/data-plane.md
- architecture/state-machines.md
- architecture/security-model.md
- design/principles.md

---

## Summary

Operations in Delta CI are intentionally conservative.

By relying on:
- explicit state machines
- leases for execution authority
- idempotent orchestration
- observable behavior

Delta CI remains operable even under partial failure.

If something is hard to operate, it’s likely a design issue—not an ops issue.
