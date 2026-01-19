# Diff-Aware Planning

This document describes how **Delta CI decides what to run** for a given change.

Diff-aware planning is the core differentiator of Delta CI.  
Instead of executing a static pipeline, the system derives an execution plan based on **what changed**, **what is affected**, and **what is required for confidence**.

---

## Goals

Diff-aware planning aims to:

- minimize unnecessary work
- provide fast and relevant feedback
- preserve correctness and safety
- remain deterministic and explainable

Skipping work is an optimization, not a risk.

---

## Inputs to Planning

The planner operates on a fixed set of inputs:

- repository structure
- detected technology stack(s)
- changed files (diff)
- explicit configuration (`ci.ai.yaml`, if present)
- persisted build recipes from previous successful runs
- system policies (required checks, limits)

No hidden state is allowed.

Phase 1 uses the local git checkout to compute diffs. The repository root is
resolved from `DELTA_CI_REPO_ROOT` (or the current working directory if unset).

---

## Planning Phases

Planning is executed in **three explicit phases**.

---

## Phase 1: Discovery

The discovery phase answers:  
**“How does this repository build and test?”**

### Sources

The planner inspects:
- well-known build files (`*.sln`, `package.json`, `go.mod`, etc.)
- workspace definitions (monorepos, workspaces)
- Makefiles / task runners
- README and CONTRIBUTING documentation
- existing persisted recipes

### Output

Discovery produces:
- detected projects and boundaries
- candidate build and test commands
- toolchain requirements
- project-to-path mappings

If discovery fails, the planner must fall back to a safe default (e.g., run all tests).

---

## Phase 2: Impact Analysis

The impact phase answers:  
**“What is affected by this change?”**

### Diff Analysis

The planner evaluates:
- changed file paths
- file types
- project ownership of files
- dependency relationships (if known)

### Impact Resolution

Rules include:
- changes inside a project affect that project
- shared libraries affect dependents
- configuration changes may affect everything
- unknown impact defaults to conservative execution

For Go monorepos, each `go.mod` is treated as a project boundary. When a
`go.work` file is present, its `use` list defines the module roots. Impacted
modules are expanded to dependents when the dependency graph is known, and job
plans are emitted per impacted module with the workdir set to the module root.

Impact analysis must err on the side of correctness.

---

## Phase 3: Plan Construction

The construction phase answers:  
**“What is the minimal safe set of jobs to run?”**

### Job Selection

Jobs may include:
- builds
- unit tests
- integration tests
- linters
- static analysis

Jobs are classified as:
- **required** (block merge)
- **allow-failure** (informational)

### Dependencies

The planner defines:
- job ordering
- parallelization
- shared cache usage

For Go module projects, `test` and `lint` jobs depend on the corresponding
`build` job for that module to enforce per-project sequencing.

The result is a directed acyclic graph (DAG) of jobs.

---

## Fallback Behavior

If any phase produces uncertainty:

- missing dependency graph
- unknown file ownership
- ambiguous tooling

The planner must **expand the plan**, not shrink it.

Correctness is always preferred over optimization.

---

## Phase 1 Minimal Heuristics

Phase 1 uses a minimal, deterministic heuristic set:

- If `go.mod` or `go.sum` is present, the planner emits Go jobs.
- The diff is computed with `git show --name-only <commit_sha>`.
- Docs-only changes (`docs/`, `README.md`, `CONTRIBUTING.md`, `*.md`) run `go build` only.
- Code or global-impact changes run `go build` and `go test`.
- A `lint` job (`go vet ./...`) is included as **allow-failure** when code is touched.
- If diff or discovery fails, the planner falls back to the static plan.

---

## Role of AI in Planning

AI may assist with:

- interpreting repository documentation
- proposing candidate commands
- suggesting project boundaries
- explaining plan decisions

AI must NOT:
- execute discovery steps
- override explicit configuration
- reduce the plan without justification

All AI outputs must be explainable and auditable.

---

## Explainability Requirements

For every plan, the system must be able to explain:

- which files triggered which jobs
- why jobs were included or excluded
- why a fallback was chosen
- what assumptions were made

Explainability is not optional.

---

## Recipe Persistence

After a successful run, Delta CI may persist a **working recipe**:

- discovered commands
- tool versions
- cache keys
- artifact paths

Recipes are versioned and tied to repository fingerprints.

Persisted recipes:
- accelerate future planning
- reduce repeated discovery
- improve determinism

Recipes never override explicit configuration.

---

## Explicit Configuration Override

If `ci.ai.yaml` is present:

- it becomes the authoritative source
- auto-detection is bypassed or limited
- planner enforces declared behavior

Diff-aware optimization may still be applied *within* declared constraints.

---

## Safety Rules

The planner must never:
- skip required checks by default
- introduce deployment steps
- modify repository state
- depend on non-deterministic input

Violations are considered critical bugs.

---

## Planning Failures

If planning fails due to:
- parser errors
- repository access issues
- internal errors

The system must:
- mark planning as failed
- fail the run explicitly
- provide a clear explanation

Silent fallback is forbidden.

---

## Related Documents

- design/principles.md
- architecture/overview.md
- architecture/components.md
- design/recipes-and-discovery.md
- adr/ADR-0002-why-diff-aware-ci.md

---

## Summary

Diff-aware planning is how Delta CI delivers fast feedback without sacrificing safety.

By separating discovery, impact analysis, and plan construction—and by favoring explicit fallbacks—the system ensures that optimizations never compromise correctness.
