# Caching Strategy

This document describes how **Delta CI uses caching** to reduce redundant work while preserving correctness and reproducibility.

Caching is an optimization.  
Incorrect caching is worse than no caching.

Delta CI’s caching model is explicit, bounded, and explainable.

---

## Goals

Caching in Delta CI aims to:

- reduce dependency download time
- accelerate repeated builds and tests
- avoid unnecessary recomputation
- preserve correctness under change

Caching must never:
- introduce nondeterminism
- hide failures
- leak data between jobs or projects

---

## Cache Scope and Ownership

### Cache Scope

Caches are scoped by:
- repository
- job definition
- cache type
- explicit cache key

There is no global, implicit cache.

---

### Cache Ownership

- caches are created and consumed by runners
- cache validity is decided by the control plane
- runners do not infer cache correctness

The control plane defines *what may be cached* and *under which conditions*.

---

## Cache Types

Delta CI supports multiple cache types.

### 1. Dependency Cache

Used for:
- language package managers
- build tool dependencies

Examples:
- npm / pnpm / yarn caches
- NuGet packages
- Maven / Gradle caches
- Go module cache

Characteristics:
- read-write
- keyed by lockfiles and tool versions
- safe to reuse across jobs of the same repo

---

### 2. Build Cache

Used for:
- compiled outputs
- intermediate artifacts

Examples:
- compiler caches (ccache-like)
- build system caches (Gradle, Bazel)

Characteristics:
- optional
- tool-specific
- stricter invalidation rules

Build caches must be explicitly enabled.

---

### 3. Toolchain Cache

Used for:
- SDKs
- compilers
- runtime installations

Characteristics:
- read-only
- versioned
- shared across runners when possible

Toolchain caches must never include secrets.

---

## Cache Key Design

Cache keys must be:

- deterministic
- explicit
- explainable
- collision-resistant

### Recommended Inputs

Cache keys may include:
- lockfile hashes
- dependency manifest hashes
- toolchain version identifiers
- job name or role

Example:
```
deps:nuget:{lockfile_hash}:sdk9
```

---

### Forbidden Inputs

Cache keys must not include:
- timestamps
- random values
- branch names (unless explicitly intended)
- environment-specific paths

---

## Cache Invalidation

Cache invalidation is explicit.

Caches are invalidated when:
- cache key changes
- repository fingerprint changes
- toolchain version changes
- explicit cache clear is requested

There is no automatic “guessing” of cache validity.

---

## Cache Lifecycle

1. Control plane defines cache configuration
2. Runner mounts cache volume
3. Job executes using cache
4. Cache is updated (if allowed)
5. Cache persists beyond job lifetime

Caches must be safe to delete at any time.

---

## Cache Isolation

Rules:
- caches must not contain secrets
- caches must not contain source code
- caches must not leak data across repositories
- fork PRs must use read-only caches or no cache

Isolation is enforced structurally, not by convention.

---

## Fork PR Policy

For forked pull requests:
- dependency caches may be mounted read-only
- build caches are disabled by default
- no cache writes are allowed

This prevents cache poisoning.

---

## Cache Failures

If cache access fails:
- job continues without cache
- failure is logged
- no retry is triggered solely due to cache failure

Cache failure must not break correctness.

---

## Observability

For each job, Delta CI must be able to report:
- which caches were used
- cache hit/miss status
- cache keys
- cache size (if available)

Cache behavior must be transparent.

---

## Performance Tradeoffs

Delta CI prefers:
- correctness over speed
- explicit over implicit caching
- fewer safe caches over many risky caches

Aggressive caching without explainability is discouraged.

---

## Security Considerations

- caches are treated as untrusted input
- caches must be validated by tools consuming them
- cache contents must not be executed blindly

Cache poisoning risks must be mitigated by policy.

---

## Non-Goals

Caching does not aim to:
- guarantee reproducible builds
- replace build system caching
- optimize every workflow

Delta CI integrates with existing tools; it does not replace them.

---

## Related Documents

- design/principles.md
- design/diff-aware-planning.md
- architecture/data-plane.md
- architecture/security-model.md
- reference/configuration.md

---

## Summary

Caching in Delta CI is explicit, bounded, and conservative.

By requiring deterministic keys, strict isolation, and clear observability, Delta CI ensures that caching accelerates work without undermining correctness or security.