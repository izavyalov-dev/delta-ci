# Delta CI

**Delta CI** is an open-source, **diff-aware, AI-assisted continuous integration system** designed to reduce wasted compute, improve feedback quality, and make CI behavior explainable.

Delta CI does not ask “what pipeline should I run?”  
It asks **“what actually changed, and what really needs to run?”**

---

## Why Delta CI?

Traditional CI systems:
- run the same static pipelines on every change
- waste compute on unaffected code paths
- produce noisy, slow feedback 
- treat failures as logs to read, not problems to understand

Delta CI takes a different approach.

---

## Core Ideas

### 🔹 Diff-Aware by Design
Jobs are selected based on:
- changed files
- project boundaries
- dependency impact

Not every change needs a full rebuild.

---

### 🔹 Deterministic Orchestration
- explicit state machines
- lease-based execution fencing
- idempotent, recoverable behavior

Correctness first. Always.

---

### 🔹 Control Plane / Data Plane Split
- **Control Plane** decides and tracks state
- **Data Plane** executes untrusted code

This enables:
- strong security boundaries
- predictable recovery
- scalable execution

---

### 🔹 AI as an Assistant (Not an Authority)
AI is used to:
- explain failures
- assist discovery
- help humans understand what happened

AI never:
- applies changes automatically
- deploys code
- bypasses policies

Human-in-the-loop is mandatory.

---

## What Delta CI Is (and Is Not)

### Delta CI **is**
- open source
- self-host friendly
- conservative by design
- explainable and auditable
- suitable for monorepos

### Delta CI **is not**
- a deployment or CD system
- an autonomous AI agent
- a replacement for build tools
- a YAML-first pipeline DSL

---

## High-Level Architecture
```
     +--------------------+
     |    Control Plane   |
     |--------------------|
     | API / Orchestrator |
     | Planner            |
     | State Machines     |
     | Failure Analysis   |
     +----------+---------+
                |
     lease / heartbeat / complete
                |
     +----------v---------+
     |     Data Plane     |
     |--------------------|
     | Ephemeral Runners  |
     | Isolated Execution |
     | Logs & Artifacts   |
     +--------------------+
```

---

## Project Status

🚧 **Early / Design-Driven Phase**

- architecture and protocols are defined
- documentation is treated as a first-class artifact
- implementation is starting with dogfooding in mind

If something is undocumented, it should be considered undefined.

---

## Documentation

Full documentation lives in `docs/`:

- architecture and protocols
- design principles
- operations and recovery
- reference contracts
- ADRs (Architectural Decision Records)

Start here:
```
docs/README.md
```

---

## Technology Stack (Summary)

- Go (control plane and runners)
- PostgreSQL (authoritative state)
- at-least-once queue (Postgres / Redis)
- S3-compatible artifact storage
- HTTP + JSON APIs
- Prometheus + OpenTelemetry

See:
```
docs/architecture/technology-stack.md
docs/adr/ADR-0006-technology-stack.md
```

---

## Contributing

Delta CI values:
- correctness over cleverness
- explicit design over implicit behavior
- documentation before implementation

Before contributing:
- read the architecture docs
- check existing ADRs
- document design changes

A `CONTRIBUTING.md` will follow.

---

## License

Delta CI is licensed under the **Apache License 2.0**.

---

## Philosophy

> CI should scale with understanding, not just with hardware.

Delta CI exists to make CI:
- faster
- quieter
- safer
- easier to reason about

If that resonates with you — welcome.