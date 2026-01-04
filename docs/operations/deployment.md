# Deployment

This document describes deployment models for **Delta CI**.

Delta CI is designed to be self-host friendly. Deployments should be possible in:
- a single-node developer setup
- a small team server
- Kubernetes

This document focuses on deployment topology and operational assumptions, not vendor-specific scripts.

---

## Deployment Models

### 1) Local Development (Single Node)

Use case:
- contributor development
- protocol testing
- dogfooding in a small repo

Typical topology:
- control plane services running as processes or containers
- a local database
- a local queue
- a local artifact store (filesystem or S3-compatible)

Recommended approach:
- Docker Compose for one-command startup
- local runner uses the same lease protocol

Expected constraints:
- no high availability
- limited isolation (depends on runner mode)
- suitable for development only

---

### 2) Self-Hosted (Single Server / VM)

Use case:
- small org
- early adopters
- cost-efficient deployment

Topology:
- control plane services on one VM
- externalized storage (recommended): S3-compatible artifact store
- local queue + DB (or managed equivalents)

Runners:
- can run on the same machine (low isolation)
- or on separate machines (preferred)

Key choices:
- isolate runners from control plane network
- restrict runner outbound access
- use short-lived credentials for artifact upload

---

### 3) Kubernetes (Recommended for Scale)

Use case:
- multi-team deployments
- predictable scaling
- stronger isolation

Topology:
- control plane services as Deployments
- DB (managed or StatefulSet)
- queue (managed or StatefulSet)
- artifact store (managed S3-compatible preferred)
- runners as ephemeral Jobs or Pods

Advantages:
- horizontal scaling
- clear resource limits
- better fault isolation

---

## Required Infrastructure

Delta CI requires:

1. **Database**
   - run/job/lease state
   - audit events (recommended)
2. **Queue**
   - job dispatch
   - optional cancel/control channel
3. **Artifact Store**
   - logs and test reports
   - build artifacts (optional)
4. **Secrets Manager**
   - recommended for production deployments

All components must be replaceable without changing protocol semantics.

---

## Environment Separation

Recommended environments:
- `dev`
- `staging`
- `prod`

Rules:
- do not reuse secrets between environments
- do not share caches between environments
- isolate runners per environment

---

## Security Defaults

Production deployments should enforce:
- mTLS or signed tokens for runner ↔ orchestrator
- restricted outbound networking from runners
- no secrets for fork PRs
- immutable runner images
- artifact store write access only from runners (scoped)

---

## Configuration Surface (Operational)

Operators should be able to configure:
- max concurrent runners
- job timeouts and run timeouts
- queue visibility timeout / lease TTL alignment
- artifact retention policies
- per-repo policies (allowed images, allowed egress, etc.)

Exact config format is implementation-defined.

---

## Rollout Strategy

Recommended rollout approach:
- start with a single repo dogfooding
- enable conservative planning (run more, not less)
- enable caching later
- enable AI features last (after safety guardrails are proven)

---

## Future Work

Planned additions:
- Helm chart for Kubernetes deployments
- hardened runner templates (container + VM)
- official Docker Compose for local development