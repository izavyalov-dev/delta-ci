# ADR-0007: Language Plugin System

**Status:** Accepted
**Date:** 2026-02-17

---

## Context

Delta CI's diff-aware planner was originally hardcoded to Go. The `diff_planner.go` mixed language-agnostic logic (diff analysis, impact analysis, dependency graph resolution) with Go-specific code (go.mod parsing, `go build`/`go test` job generation).

To support additional language ecosystems (Rust, Node.js, Python, C#, etc.) without requiring every contributor to modify core planner code, we need a plugin architecture that separates language-specific concerns from the diff-aware planning engine.

### Requirements

1. Adding a new language must not require forking or modifying core planner code
2. Go support must remain a first-class, built-in plugin
3. Community plugins should be writable in any language
4. Plugin failure must never silently skip work (conservative fallback)
5. Multiple languages in one repository must be supported

---

## Decision

Introduce a `LanguagePlugin` interface with three operations:

- **Detect** -- does this repository use my language? (quick marker-file check)
- **Discover** -- full project discovery: projects, dependencies, code extensions
- **Plan** -- given impacted projects, produce language-specific jobs

### Two execution modes

1. **In-process** -- Go structs compiled into the binary. The Go plugin uses this mode.
2. **External** -- standalone executables (`delta-ci-lang-<name>` in PATH) communicating via JSON over stdin/stdout. Community plugins use this mode and can be written in any language.

### Plugin dispatch flow

```
1. DetectAll(plugins)              -- which languages are present?
2. For each detected plugin:
     Discover()                    -- projects, code extensions, global files
3. Merge all projects into discoveryInputs
4. analyzeImpact(paths, merged)    -- UNCHANGED, language-agnostic
5. For each detected plugin:
     Plan(impacted)                -- language-specific jobs
6. Merge all jobs into PlanResult
```

### Safety invariants

- Plugin detection failure is treated as "detected" (conservative)
- Plugin plan failure triggers a conservative fallback (expand plan, never skip)
- External plugins get a 30-second timeout (configurable)
- Plugins receive only repo root path and structural data; never secrets, tokens, or lease IDs
- All detected plugins fire; no early exit on first match

---

## Alternatives Considered

### Language detection via configuration file

Require users to declare languages in `ci.ai.yaml`. Rejected because:
- Adds friction for new users (Delta CI's value prop is zero-config)
- Breaks the "just push and it works" experience

### gRPC/HTTP plugin protocol

More structured but adds deployment complexity (port allocation, process lifecycle). The stdin/stdout JSON protocol matches git's credential-helper model, which is simpler and well-understood.

### Single monolithic planner with language switches

Simpler initially but does not scale to community contributions. Every new language would require a PR to core, creating a maintenance bottleneck.

---

## Consequences

### Positive

- Community can add language support without modifying core planner code
- Go support is preserved as a zero-config built-in
- Multi-language monorepos work out of the box
- External plugins follow a simple, well-documented JSON protocol
- Conservative fallback ensures plugin bugs never silently skip CI work

### Negative

- External plugin execution adds latency (~30ms-300ms per invocation)
- Plugin protocol versioning must be maintained for backward compatibility
- More code surface area to test and maintain

### Neutral

- Existing `analyzeImpact`, `buildDependencyGraph`, and other core functions remain unchanged
- All existing tests continue to pass without modification
