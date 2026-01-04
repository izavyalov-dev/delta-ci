# Disaster Recovery

This document describes failure and recovery scenarios for Delta CI.

Delta CI assumes failure is normal. Recovery procedures must be explicit, repeatable, and safe.

---

## Recovery Objectives

- preserve authoritative state (runs/jobs/leases)
- avoid corrupting state via late or duplicate messages
- minimize operator intervention
- ensure clear audit trails

---

## Critical Dependencies

Delta CI depends on:
- Database (authoritative state)
- Queue (dispatch and backpressure)
- Artifact store (logs and outputs)
- Secrets manager (production)

Availability of these dependencies defines recovery strategy.

---

## Failure Scenarios

### 1) Orchestrator Crash / Restart

Expected behavior:
- orchestrator restarts and loads state from DB
- in-flight leases remain valid until TTL
- missing heartbeats cause lease expirations and retries

Operator actions:
- none required in normal cases
- investigate repeated crashes via logs/metrics

---

### 2) Runner Crash

Expected behavior:
- lease expires
- job attempt returns to queued state (if retries allowed)
- run continues

Operator actions:
- none unless crash rate spikes
- check runner image, resource limits, node stability

---

### 3) Queue Outage

Impact:
- new jobs cannot be dispatched
- running jobs may still complete and upload artifacts
- system may appear stalled

Recovery:
- restore queue
- orchestrator may re-enqueue jobs that were queued but not leased
- ensure idempotency prevents duplicates from corrupting state

Operator actions:
- verify queue health
- check backlog and job age
- consider pausing new run ingestion if outage is prolonged

---

### 4) Database Outage

Impact:
- orchestrator cannot transition state
- leasing and completion cannot be finalized safely
- system must fail closed

Recovery:
- restore DB
- resume orchestrator
- allow leases to expire naturally
- orchestrator reconciles queued vs active jobs

Operator actions:
- restore DB from backups if needed
- validate DB integrity
- review audit logs for partial updates

---

### 5) Artifact Store Outage

Impact:
- logs and reports may not upload
- job correctness may still be determinable by exit code
- explainability degrades

Recovery:
- restore artifact store
- runners may retry uploads if supported
- mark missing artifacts explicitly

Operator actions:
- restore storage
- verify retention and permissions
- ensure artifact URIs are not broken

---

### 6) Mass Lease Expirations

Symptoms:
- many jobs re-queued unexpectedly
- stale completions increase
- queue depth spikes

Common causes:
- orchestrator latency / DB slowness
- network partitions
- runner node overload

Recovery:
- stabilize DB and orchestrator latency
- consider temporarily increasing TTL
- reduce concurrency to restore steady state

Operator actions:
- inspect orchestrator and DB metrics
- check runner node health
- apply rate limits if needed

---

## Backups and Restore

### Database
- periodic backups (daily minimum; more frequent for active installs)
- point-in-time recovery recommended
- test restores regularly

### Artifact Store
- lifecycle policies and replication if needed
- artifacts are not authoritative state, but useful for forensics

---

## Data Integrity Rules During Recovery

- never accept `Complete` for stale leases
- never roll back terminal states
- reconcile using state machine rules only
- prefer retry over manual state edits

Manual DB edits are a last resort and must be audited.

---

## Runbook Checklist (Minimal)

When the system is unhealthy:

1. Check DB health and latency
2. Check queue depth and age
3. Check orchestrator error rate
4. Check runner availability
5. Check lease expiration rate
6. Check artifact upload failures

Take action based on the failing dependency.

---

## Summary

Delta CI is designed to recover via:
- persistent state
- lease expiration
- idempotent dispatch
- bounded cancellation and retries

Disaster recovery should focus on restoring dependencies and letting the state machines do the rest.