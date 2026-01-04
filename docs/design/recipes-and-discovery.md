# Recipes and Discovery

This document describes how **Delta CI discovers how a repository builds** and how it **persists working build recipes** over time.

Discovery and recipes exist to reduce repeated guessing, improve determinism, and gradually move projects toward explicit contracts—without requiring upfront configuration.

---

## Goals

The discovery and recipe system aims to:

- enable zero-config onboarding
- converge toward stable, repeatable builds
- reduce planning uncertainty over time
- preserve correctness and safety
- avoid hidden or magical behavior

Discovery is adaptive; recipes are explicit.

---

## Definitions

- **Discovery**: The process of inferring how a repository should be built and tested.
- **Recipe**: A persisted, versioned description of a working build/test approach.
- **Repository Fingerprint**: A hash representing the structural identity of a repository.

---

## Discovery Philosophy

Discovery is a **best-effort inference mechanism**, not a source of truth.

Key principles:
- discovery is conservative
- discovery favors safety over speed
- discovery must be explainable
- discovery results are not silently mutated

If discovery cannot reach sufficient confidence, Delta CI must fall back to a safe default.

---

## Discovery Sources

The planner may inspect the following sources during discovery:

### Build Files
- `.sln`, `.csproj`
- `package.json`
- `go.mod`
- `pom.xml`, `build.gradle`
- `Cargo.toml`
- `WORKSPACE`, `BUILD.bazel`
- `Dockerfile`

### Task Runners
- `Makefile`
- `justfile`
- `Taskfile.yml`
- npm/pnpm/yarn scripts

### Repository Metadata
- directory layout
- workspace definitions (monorepos)
- dependency manifests

### Documentation
- `README.md`
- `CONTRIBUTING.md`
- build/test sections

Documentation is advisory and must be validated by execution.

---

## Discovery Output

Discovery produces a **candidate build description**, including:

- detected projects and boundaries
- candidate commands (build, test, lint)
- required toolchains and versions
- artifact paths
- cacheable directories

This output is **not executed blindly**.

---

## Validation Phase

All discovered commands must be validated.

Validation rules:
- start with safe commands only
- no deployment steps
- no infrastructure changes
- no privileged access

Typical validation steps:
- dependency restore/install
- build
- unit tests

If validation fails, discovery must:
- try alternative safe candidates (bounded)
- or fall back to conservative execution

---

## Recipe Creation

A **recipe** is created only after a successful, validated run.

A recipe includes:
- job definitions
- execution order
- working directories
- cache keys and paths
- artifact definitions
- toolchain requirements

Recipes represent **known-good behavior**, not guesses.

---

## Recipe Persistence

Recipes are persisted with:

- repository identifier
- repository fingerprint
- recipe version
- creation timestamp
- last successful usage timestamp

Recipes are immutable once created.

---

## Repository Fingerprint

A fingerprint represents the structural identity of a repository.

It may include:
- hashes of build files
- workspace definitions
- dependency manifests

Fingerprint changes indicate that:
- discovery may need to be re-run
- existing recipes may no longer be valid

---

## Recipe Selection

When planning:

1. Check for explicit configuration (`ci.ai.yaml`)
2. If absent, check for matching persisted recipe
3. If fingerprint mismatch, run discovery
4. If discovery fails, fall back to safe defaults

Recipes never override explicit configuration.

---

## Recipe Versioning

Recipes are versioned to allow evolution.

Reasons for new recipe versions:
- repository structure change
- tooling change
- improved discovery logic

Older recipes are retained for auditability.

---

## Recipe Transparency

For every run using a recipe, Delta CI must be able to explain:

- which recipe was used
- why it was selected
- when and how it was created
- what assumptions it encodes

Recipes are visible to users.

---

## Proposing Explicit Configuration

Delta CI may suggest adding `ci.ai.yaml` when:

- discovery converges repeatedly
- recipes stabilize
- teams want explicit control

This suggestion must be:
- optional
- non-blocking
- delivered as a PR or patch proposal

Delta CI never auto-commits configuration.

---

## Role of AI in Discovery

AI may assist with:
- interpreting documentation
- ranking candidate commands
- mapping project boundaries

AI must not:
- execute commands
- persist recipes
- reduce safety checks
- override validation failures

AI suggestions must be validated by execution.

---

## Failure Modes

### Discovery Failure
If discovery fails completely:
- fail planning explicitly
- explain why discovery failed
- recommend adding explicit configuration

Silent fallback is forbidden.

---

### Recipe Drift
If a recipe no longer works:
- mark recipe as stale
- trigger re-discovery
- preserve old recipe for audit

---

## Security Considerations

- discovery never executes arbitrary code
- validation is sandboxed
- recipes contain no secrets
- recipes do not grant privileges

All recipe data is treated as non-sensitive metadata.

---

## Related Documents

- design/diff-aware-planning.md
- design/principles.md
- architecture/overview.md
- architecture/security-model.md
- reference/configuration.md

---

## Summary

Discovery enables zero-config onboarding.  
Recipes enable deterministic, explainable execution.

Together, they allow Delta CI to move projects from inference to intent—without sacrificing safety or transparency.