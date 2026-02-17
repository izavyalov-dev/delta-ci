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
- use the optional AI proxy to isolate provider credentials

---

## Docker Deployment

Delta CI provides multi-stage Dockerfiles for all components:

- `Dockerfile.orchestrator` — orchestrator binary (serve, worker, dogfood modes)
- `Dockerfile.runner` — runner binary
- `Dockerfile.ai-proxy` — AI proxy binary

All images use `gcr.io/distroless/static:nonroot` as the runtime base for minimal attack surface.

### Quick Start

Build and start the full stack with Docker Compose:

```bash
# Build all images and start services
make docker-up

# Or step by step:
docker compose build
docker compose up -d
```

This starts PostgreSQL, the orchestrator API server, and a worker. The API is available at `http://localhost:8080`.

### Individual Builds

```bash
docker build -f Dockerfile.orchestrator -t delta-ci/orchestrator .
docker build -f Dockerfile.runner -t delta-ci/runner .
docker build -f Dockerfile.ai-proxy -t delta-ci/ai-proxy .
```

### Running the Orchestrator

The orchestrator binary supports three modes via the first argument:

```bash
# API server
docker run -e DATABASE_URL=... delta-ci/orchestrator serve --listen :8080

# Background worker
docker run -e DATABASE_URL=... delta-ci/orchestrator worker --orchestrator-url http://orchestrator:8080

# Local dogfood
docker run -e DATABASE_URL=... delta-ci/orchestrator dogfood
```

### Environment Variables

All CLI flags can also be set via environment variables. Key variables:

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `GITHUB_TOKEN` | GitHub API token for status checks |
| `GITHUB_WEBHOOK_SECRET` | Webhook signature verification |
| `DELTA_NOTIFY_WEBHOOK_URL` | Generic webhook notification URL |
| `DELTA_NOTIFY_SLACK_WEBHOOK_URL` | Slack incoming webhook URL |

See [notifications.md](notifications.md) for notification configuration.

---

## Future Work

Planned additions:
- Helm chart for Kubernetes deployments
- hardened runner templates (container + VM)
