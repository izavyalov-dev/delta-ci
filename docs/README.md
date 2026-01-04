# Delta CI Documentation

This directory contains the architectural, design, and operational documentation for **Delta CI**.

Delta CI is an AI-native, diff-aware continuous integration system designed to reduce noise, wasted compute, and slow feedback loops by understanding *what changed* and *what actually needs to run*.

This documentation is intended for:
- contributors
- maintainers
- reviewers
- architects evaluating or extending Delta CI

---

## How to Read This Documentation

If you are new to the project, read the documents in this order:

1. Architecture Overview
2. Core Design Principles
3. Runner Protocol and State Machines
4. Diff-Aware Planning
5. Architectural Decision Records (ADRs)

Each document is written to be readable independently, but together they form a complete picture of how Delta CI works and why it is designed this way.

---

## Documentation Structure

### Architecture

High-level and low-level architecture of the system.
```
architecture/
├─ overview.md            # High-level system overview
├─ components.md          # Core components and responsibilities
├─ control-plane.md       # Orchestrator, planner, state management
├─ data-plane.md          # Runners and execution environment
├─ runner-protocol.md     # Lease / heartbeat / complete / cancel protocol
├─ state-machines.md      # Run, job, and lease lifecycle
└─ security-model.md      # Security boundaries and trust model
```
---

### Design

Rationale and internal logic behind core system behaviors.
```
design/
├─ principles.md              # Core design philosophy
├─ diff-aware-planning.md     # How Delta CI decides what to run
├─ recipes-and-discovery.md   # Build discovery and recipe persistence
├─ ai-usage.md                # Where and how AI is used (and where it is not)
├─ failure-analysis.md        # Explaining failures and proposing fixes
└─ caching-strategy.md        # Dependency and build caching model
```
---

### Operations

Guidance for running and maintaining Delta CI installations.
```
operations/
├─ deployment.md          # Deployment models (local, k8s, self-hosted)
├─ scaling.md             # Horizontal and vertical scaling strategies
├─ observability.md       # Metrics, logs, traces
└─ disaster-recovery.md   # Failure and recovery scenarios
```
---

### Reference

Precise, contract-style documentation.
```
reference/
├─ terminology.md         # Canonical definitions of core terms
├─ api-contracts.md       # Orchestrator and external APIs
├─ runner-messages.md     # Runner protocol message schemas
└─ configuration.md       # ci.ai.yaml and policy configuration
```
---

### Architectural Decision Records (ADR)

Immutable records explaining *why* key decisions were made.
```
adr/
├─ ADR-0001-project-scope.md
├─ ADR-0002-why-diff-aware-ci.md
├─ ADR-0003-license-choice.md
├─ ADR-0004-control-vs-data-plane.md
└─ ADR-0005-runner-lease-model.md
```
ADRs should be read as historical context and design justification.  
They are not rewritten when the system evolves.

---

## Documentation Principles

Delta CI documentation follows these rules:

- Design decisions must be documented before large implementations.
- Safety and correctness take priority over convenience.
- AI behavior must always have explicit boundaries.
- Protocols and state machines must be unambiguous.
- Human-in-the-loop is a feature, not a limitation.

---

## Contributing to Documentation

Documentation is treated as part of the system.

Good contributions include:
- clarifying architecture decisions
- improving diagrams and explanations
- documenting edge cases and failure modes
- fixing ambiguities in protocols

If a behavior is not documented, it should be considered undefined.

---

## Status

The documentation reflects the **current intended architecture** of Delta CI.

Some parts may precede implementation and are expected to guide development.