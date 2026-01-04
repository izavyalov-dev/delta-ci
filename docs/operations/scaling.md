# Scaling

This document describes how Delta CI scales.

Delta CI uses a **control plane / data plane** split:
- Control plane scales with orchestration complexity
- Data plane scales with workload (jobs)

Scaling is primarily horizontal.

---

## Scaling Dimensions

### 1) Jobs per Time (Throughput)
- driven by runner capacity
- limited by queue, artifact store, and registry bandwidth

### 2) Concurrent Runs
- driven by orchestration state updates
- limited by DB throughput and status reporting

### 3) Repository Size (Monorepos)
- driven by planning cost and diff analysis
- requires careful caching of metadata and recipes

---

## Control Plane Scaling

### Orchestrator
Scale strategy:
- horizontally scale stateless instances
- single authoritative DB for state
- ensure idempotency on all state transitions

Bottlenecks:
- DB write throughput (state transitions, heartbeats)
- queue throughput (job dispatch)
- artifact metadata ingestion

Mitigations:
- batch state writes where safe
- store heartbeats in a lightweight table or time-series (optional)
- keep orchestration logic deterministic and minimal

---

### Planner
Scale strategy:
- run planners as stateless workers
- cache repository metadata and fingerprints
- limit planning time and complexity

Mitigations:
- bounded discovery attempts
- conservative fallback on ambiguity
- persist recipes to reduce repeated discovery cost

---

### Failure Analyzer
Scale strategy:
- async processing after job completion
- strict bounds on log size
- cache parsed test reports

Mitigations:
- separate queues for analysis vs execution
- circuit breakers for AI providers
- never block job finalization on analysis

---

## Data Plane Scaling

### Runner Capacity
Scale by increasing:
- number of runners
- runner concurrency (usually avoid; prefer 1 job per runner)
- runner pool diversity (capabilities)

Recommended model:
- runners are ephemeral and disposable
- scale based on queue depth and SLOs

---

## Queue and Lease Tuning

The following must align:
- queue visibility timeout
- lease TTL
- heartbeat interval

Recommended defaults:
- lease TTL: 120s
- heartbeat: 20s
- queue visibility timeout: >= lease TTL + heartbeat jitter

Goal:
- avoid premature redelivery
- allow recovery from dead runners

---

## Artifact Store Scaling

Artifacts can dominate I/O.

Recommendations:
- use S3-compatible object storage
- upload logs incrementally where possible
- compress logs and reports
- apply retention policies

---

## Database Scaling

The DB is the source of truth.

Recommendations:
- use a production-grade relational DB
- index run/job/lease lookup paths
- keep hot state small
- consider partitioning by repo or time if needed

Avoid:
- storing large logs in the DB

---

## Caching and Scale

Caching improves throughput but adds risk.

Scaling-safe rules:
- dependency caches are safest
- build caches require stricter invalidation
- fork PRs must be read-only or no-cache to prevent poisoning

---

## Metrics to Watch

- queue depth and age
- runner utilization
- lease expirations (unexpected spikes indicate instability)
- artifact upload latency
- DB write latency
- end-to-end run duration by repo

---

## Summary

Delta CI scales by:
- keeping the control plane deterministic and stateless
- scaling runners horizontally
- aligning queue and lease semantics
- pushing large data to object storage