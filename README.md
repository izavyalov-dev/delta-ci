# Delta CI

**Delta CI** is an AI-native, diff-aware continuous integration system.

Instead of running everything on every change, Delta CI understands **what changed**, decides **what needs to run**, and helps you **fix failures**, not just report them.

> CI driven by deltas, not by fear.

---

## Why Delta CI?

Traditional CI systems are:
- static (same pipeline for every change)
- noisy (run all tests, all the time)
- passive (fail → read logs → fix manually)

Delta CI is built around three ideas:

1. **Diff-aware execution**  
   Run only what is impacted by the change.

2. **AI-assisted understanding**  
   Explain *why* a job failed and *what to do next*.

3. **Minimal contracts, strong defaults**  
   Works out of the box, but lets teams take control when needed.

---

## Core Concepts

### 1. Diff-Aware Planning
Delta CI analyzes:
- changed files
- project structure
- detected tech stack(s)
- previous successful build recipes

Based on that, it generates a **run plan** instead of blindly executing a static pipeline.

Example:
- docs-only change → no build
- backend change → unit tests only
- shared library change → downstream services

---

### 2. AI-Assisted Failure Analysis
When something fails, Delta CI:
- collects logs and artifacts
- detects the likely root cause
- explains the failure in human terms
- optionally suggests a fix or a patch

AI **never blindly deploys or mutates your code** — fixes are always validated and require human approval.

---

### 3. Ephemeral & Secure Execution
- Each job runs in a short-lived sandbox
- No long-lived secrets in runners
- Fork PRs run with zero trust by default
- OIDC-based access to cloud providers

---

## Architecture Overview

Delta CI follows a **control plane / data plane** design:

- **Control Plane**
  - API & Web UI
  - Orchestrator
  - Planner (rules + AI)
  - Failure Analyzer
  - State & metadata

- **Data Plane**
  - Ephemeral runners
  - Isolated execution
  - Artifact & cache storage

This separation keeps execution scalable, secure, and observable.

---

## How It Works (High Level)

1. Git provider sends a webhook (PR / push)
2. Delta CI creates a run and analyzes the diff
3. A plan is generated (what to run and why)
4. Jobs are executed by ephemeral runners
5. Results and artifacts are collected
6. Status and explanations are reported back to the PR

---

## Repository Structure (planned)

```text
delta-ci/
├─ control-plane/
│  ├─ api/
│  ├─ orchestrator/
│  ├─ planner/
│  └─ failure-analyzer/
├─ runner/
│  ├─ agent/
│  └─ images/
├─ web/
├─ docs/
├─ examples/
└─ README.md
```

## Configuration Philosophy

Delta CI prefers **convention over configuration**.

If needed, projects can define an explicit contract:
```yaml
# ci.ai.yaml
version: 1

jobs:
  build:
    image: dotnet:9.0
    steps:
      - dotnet restore
      - dotnet build -c Release
      - dotnet test -c Release
```

If no config exists, Delta CI attempts safe auto-detection and proposes a working recipe.

## What Delta CI Is Not
	•	❌ Not a replacement for every CI feature on day one
	•	❌ Not a YAML-heavy pipeline generator
	•	❌ Not an autonomous deployment bot

Delta CI focuses on build correctness, signal quality, and developer feedback loops.

## Project Status
🚧 Early development / design-driven stage

The project is currently:
	•	stabilizing core architecture
	•	defining execution and runner protocols
	•	implementing a minimal working CI loop
	•	dogfooding on itself

APIs and internals will change.

## Roadmap (Short Term)
	•	GitHub integration (checks + PR comments)
	•	Diff-aware planner MVP
	•	Ephemeral runner protocol (lease / heartbeat)
	•	Artifact & log storage
	•	AI failure explanation (read-only)
	•	Self-hosted deployment docs

## Philosophy

> CI should help you merge with confidence,
> not punish you for touching code.

Delta CI exists to reduce wasted compute, noisy feedback,
and slow developer loops — without hiding complexity when it matters.
