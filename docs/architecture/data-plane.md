# Data Plane

This document describes the **Data Plane** of Delta CI.

The data plane is responsible for **executing jobs and producing artifacts**.  
It is intentionally isolated, disposable, and treated as **untrusted** by design.

---

## Responsibilities

The Data Plane is responsible for:

- executing jobs defined by the control plane
- providing isolated execution environments
- producing logs and artifacts
- reporting execution results via the runner protocol
- enforcing execution limits (time, resources)

The Data Plane does **not**:
- make orchestration decisions
- persist authoritative state
- communicate with VCS systems
- access long-lived secrets

---

## Core Components

### Runner Controller

**Purpose**
- provision and manage execution capacity

**Responsibilities**
- start and stop runner instances
- scale runners up and down
- match jobs to runner capabilities (OS, arch, toolchains)
- enforce quotas and concurrency limits

**Characteristics**
- stateless or lightly stateful
- reacts to queue depth and demand
- does not understand job semantics

**Does NOT**
- track job state
- decide retries or cancellations
- interpret job results

---

### Runner

**Purpose**
- execute exactly one job at a time

**Key Characteristics**
- ephemeral (short-lived)
- single-job ownership
- isolated filesystem
- restricted network
- no persistent credentials

**Lifecycle**
1. start
2. register capabilities
3. lease a job
4. execute job steps
5. upload logs and artifacts
6. report completion
7. terminate

Runners are disposable and replaceable at any time.

---

## Execution Environment

### Isolation Model

Runners must be isolated using one of:
- containers with hardened profiles (seccomp, AppArmor)
- lightweight virtual machines

Minimum guarantees:
- no shared writable filesystem between runners
- no shared process namespace
- no shared credentials

---

### Filesystem

- each runner starts with a clean workspace
- repository code is cloned or mounted per job
- caches are explicitly mounted and scoped
- workspace is destroyed after job completion

Caches must never contain secrets.

---

### Network Access

Default network policy:
- deny all inbound connections
- restrict outbound connections

Allowed outbound access may include:
- VCS endpoints
- package registries
- artifact storage

Network rules must be explicit and auditable.

---

### Resources and Limits

Runners must enforce:
- CPU limits
- memory limits
- disk quotas
- job runtime limits

Resource exhaustion must result in controlled job failure.

---

## Job Execution Model

### Step Execution

Jobs are executed as an ordered list of steps.

Rules:
- steps execute sequentially
- a non-zero exit code fails the job
- steps may emit logs and artifacts
- environment variables are scoped to the job

The runner does not interpret step semantics.

---

### Environment Variables

Runners may expose:
- system-provided variables (CI=true, RUN_ID, JOB_ID)
- explicitly allowed secrets (non-fork runs only)

Runners must never:
- inject secrets into fork PR jobs
- log environment variables by default

---

## Logging and Artifacts

### Logs

- logs are streamed or buffered during execution
- logs are uploaded to the artifact store
- logs are considered untrusted input

Log upload must be:
- incremental when possible
- resumable if supported by storage

---

### Artifacts

Artifacts may include:
- build outputs
- test reports
- coverage data
- metadata

Artifact handling rules:
- artifacts are immutable once uploaded
- artifact references are passed to the control plane
- artifacts must not be executed by control plane components

---

## Interaction With Control Plane

### Protocol

All communication with the control plane uses the **runner protocol**:
- lease
- heartbeat
- complete
- cancel

Runners must follow protocol rules strictly.
See `runner-protocol.md` for details.

---

### Failure Reporting

Runners report:
- exit codes
- summaries
- artifact references

Runners do not:
- decide retry eligibility
- mark jobs as succeeded or failed definitively

---

## Failure Scenarios

### Runner Crash

If a runner crashes:
- no heartbeat is sent
- lease expires
- job attempt is retried if allowed

Runner crashes are expected and tolerated.

---

### Network Failure

If the runner loses connectivity:
- heartbeats may fail
- lease may expire
- job completion may be rejected as stale

Runners must handle stale lease responses gracefully.

---

### Partial Execution

If a job is canceled or interrupted:
- runner should attempt to upload partial logs
- runner should send `CancelAck` if possible
- control plane decides final job state

---

## Scaling Model

Scaling is horizontal.

- more runners = more parallel jobs
- scaling decisions are based on queue depth and policies
- runners are interchangeable

The system does not rely on long-lived workers.

---

## Security Considerations

- runners are untrusted
- all runner input is validated by control plane
- lease IDs are treated as secrets
- runner images must be reviewed and versioned

Security is enforced by isolation and protocol design.

---

## Non-Goals

The Data Plane does not:
- guarantee build determinism
- optimize build performance
- provide debugging tools beyond logs/artifacts
- retain state across jobs

---

## Related Documents

- overview.md
- components.md
- control-plane.md
- runner-protocol.md
- state-machines.md
- security-model.md

---

## Summary

The Data Plane executes work and nothing more.

By keeping runners ephemeral, isolated, and stateless, Delta CI ensures:
- predictable behavior
- strong failure containment
- scalable execution
- a clear trust boundary

All intelligence and authority remain in the control plane.