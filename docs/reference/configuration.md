# Configuration

This document describes how **Delta CI is configured** using explicit, versioned configuration files.

Configuration exists to **declare intent**, not to encode logic.  
Delta CI is designed to work with minimal or no configuration, but provides configuration as an override mechanism when teams need control.

---

## Configuration Philosophy

Delta CI follows these rules:

- configuration is optional
- configuration is explicit
- configuration overrides discovery
- configuration must be explainable
- configuration must be versioned with the repository

Configuration is a contract between the repository and the CI system.

---

## Configuration File

### File Name

Delta CI looks for the following file at the repository root:
```
ci.ai.yaml
```

If present, this file becomes the **authoritative source of execution intent**.

---

## Configuration Versioning

Every configuration file must declare a version.

Example:

```yaml
version: 1
```

Versioning allows:
* backward-compatible evolution
* controlled breaking changes
* clear migration paths

## Top-Level Structure

```yaml
version: 1

jobs:
  <job-name>:
    image: <runner-image>
    workdir: <path>
    steps:
      - <command>
    artifacts: []
    caches: []
    policy: {}

policies:
  runs: {}
  jobs: {}

limits: {}
```

All sections are optional unless explicitly stated.

## Jobs

### Job Definition

A job represents a unit of work.

Example:
```yaml
jobs:
  unit-tests:
    image: ghcr.io/delta-ci/runner-dotnet:9
    workdir: .
    steps:
      - dotnet restore
      - dotnet test -c Release
```

### Job Fields

#### image (required)

Runner image used to execute the job.
*	must be explicitly defined
*	must be immutable (tagged versions recommended)

#### workdir (optional)

Working directory for job execution.

Default:
```
.
```

#### steps (required)
Ordered list of shell commands.

Rules:
*	executed sequentially
*	non-zero exit code fails the job
*	no implicit retries

#### artifacts (optional)
Defines artifacts to collect.

Example:
```yaml
artifacts:
  - type: junit
    path: "**/TestResults/*.trx"
```
Artifacts are immutable once uploaded.

#### caches (optional)
Defines cache usage.

Example:
```yaml
caches:
  - type: deps
    key: "nuget:{lock_hash}"
    paths:
      - "~/.nuget/packages"
```
Cache keys must be deterministic.

#### policy (optional)
Per-job policy overrides.

Example:
```yaml
policy:
  required: true
  allow_failure: false
  retries: 1
  timeout_seconds: 1800
```

### Job Policies

#### required

Whether job failure blocks the run.

Default:
```
true
```

#### allow_failure

Whether failure is informational only.

Default:
```
false
```

#### retries

Maximum retry attempts.

Default:
```
0
```

#### timeout_seconds

Maximum execution time for the job.

If exceeded, job is terminated and marked failed.

## Global Policies

Global policies apply across the entire run.

Example:
```yaml
policies:
  runs:
    cancel_superseded: true
    max_runtime_seconds: 3600
```

### Supported Run Policies
*	cancel_superseded
*	max_runtime_seconds
*	max_concurrency

## Limits

Limits define resource boundaries.

Example:
```yaml
limits:
  cpu: "2"
  memory: "4Gi"
  disk: "20Gi"
```
Limits are enforced by the data plane.

## Interaction With Discovery

When configuration is present:
*	discovery is skipped or constrained
*	recipes are not used
*	planner enforces declared jobs

Diff-aware optimization may still apply within declared jobs if allowed.

## Validation Rules

Configuration is validated before execution.

Invalid configuration:
*	fails the run explicitly
*	produces a clear error message
*	does not fall back silently

## Security Constraints

Configuration must never:
*	include secrets
*	reference environment-specific credentials
*	bypass security policies
*	enable deployments implicitly

Secrets must be managed via the Secrets Manager.

## Migration and Evolution

Breaking configuration changes require:
*	new configuration version
*	migration documentation
*	explicit opt-in

Older versions must remain supported for a defined period.

## Related Documents
*	design/principles.md
*	design/recipes-and-discovery.md
*	design/diff-aware-planning.md
*	architecture/security-model.md

## Summary

Configuration in Delta CI is an explicit declaration of intent.

It exists to:
*	override discovery
*	codify expectations
*	reduce ambiguity

Configuration complements automation — it does not replace it.