# ADR-0003: License Choice

**Status:** Accepted  
**Date:** 2026-01-04  

---

## Context

Delta CI is an open-source infrastructure project intended to be:

- widely adoptable
- friendly to enterprise usage
- safe for commercial environments
- attractive to contributors
- extensible without legal friction

Choosing the correct license is a **foundational decision** that directly impacts adoption, contribution, and long-term sustainability.

An overly restrictive license can:
- discourage enterprise usage
- prevent integration into existing systems
- reduce contributor participation

An overly permissive license without protections can:
- expose contributors to patent risk
- reduce trust in commercial environments

---

## Decision

Delta CI is licensed under the **Apache License, Version 2.0**.

---

## Rationale

### 1. Enterprise Compatibility

Apache 2.0 is:
- well-understood by legal teams
- commonly approved for internal and commercial use
- compatible with SaaS offerings

This lowers friction for:
- adoption
- evaluation
- contribution

---

### 2. Patent Protection

Apache 2.0 includes an explicit **patent grant**.

This:
- protects contributors
- protects users
- reduces legal uncertainty around infrastructure and protocol design

For a CI system involving orchestration, scheduling, and AI-assisted logic, this protection is important.

---

### 3. Ecosystem Alignment

Many successful infrastructure projects use Apache 2.0, including:
- Kubernetes
- Apache Kafka
- Apache Airflow
- Apache Spark

Aligning with this ecosystem:
- sets clear expectations
- signals seriousness and maturity
- avoids license friction with dependencies

---

### 4. Future Flexibility

Apache 2.0 allows:
- self-hosted deployments
- managed SaaS offerings
- commercial extensions
- dual-licensing strategies if ever required

The license does not constrain future business models.

---

## Non-Goals

The license choice does not aim to:
- force downstream projects to open-source their code
- prevent commercial usage
- encode ideological positions

The goal is **maximum adoption with reasonable protection**.

---

## Alternatives Considered

### MIT License

**Rejected because:**
- no explicit patent grant
- weaker contributor protection
- less preferred for complex infrastructure projects

MIT is suitable for libraries; Delta CI is a platform.

---

### GPL / AGPL

**Rejected because:**
- viral copyleft discourages enterprise adoption
- SaaS usage becomes legally complex
- limits integration into existing CI ecosystems

This conflicts with adoption goals.

---

### MPL 2.0

**Considered but not chosen because:**
- more complex compliance requirements
- less common in CI / infra tooling
- limited additional benefit over Apache 2.0

---

## Consequences

### Positive

- high adoption potential
- enterprise trust
- contributor protection
- ecosystem compatibility

---

### Negative

- allows proprietary forks
- does not force contributions upstream

These tradeoffs are accepted.

---

## Related Decisions

- ADR-0001: Project Scope and Goals
- ADR-0002: Why Diff-Aware CI
- ADR-0004: Control Plane vs Data Plane

---

## Summary

The Apache License 2.0 provides the best balance between:

- openness
- protection
- adoption
- long-term flexibility

This license choice supports Delta CI’s goal of becoming a widely usable, trustworthy CI foundation.