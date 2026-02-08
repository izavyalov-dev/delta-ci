# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Delta CI is an open-source, diff-aware, AI-assisted CI system written in Go. It reduces wasted compute by understanding what changed and what actually needs to run. The project is in early/design-driven phase (currently Phase 5: Web Dashboard & Management UI).

## Build & Test Commands

```bash
# Build all binaries
go build ./cmd/...

# Build individual binaries
go build -o bin/orchestrator ./cmd/orchestrator
go build -o bin/runner ./cmd/runner
go build -o bin/ai-proxy ./cmd/ai-proxy

# Run all tests
go test ./...

# Run tests for a specific package
go test ./orchestrator
go test ./planner/...

# Run a single test by name
go test -v -run TestPlanForGo ./planner/...

# Race detection
go test -race ./...

# Format and vet
go fmt ./...
go vet ./...
```

## Architecture

### Control Plane / Data Plane Split

- **Control Plane** (`orchestrator/`, `planner/`): Decides what to run, tracks state, handles retries. Never executes user code.
- **Data Plane** (`runner/`): Executes untrusted jobs in isolation. Communicates only via lease/heartbeat/complete/cancel messages.
- **Protocol** (`protocol/`): JSON message definitions between runner and orchestrator.
- **State** (`state/`): PostgreSQL persistence layer with SQL migrations in `state/migrations/`.
- **Web** (`web/`): Server-rendered dashboard using Go templates + htmx + Pico CSS, embedded via `embed.FS`.

### Entry Points

- `cmd/orchestrator/main.go` — Three modes: `serve` (HTTP API), `dogfood` (local test), `worker` (process queued jobs)
- `cmd/runner/main.go` — Executes individual jobs under a lease
- `cmd/ai-proxy/main.go` — OpenAI proxy with rate limiting and prompt sanitization

### State Machines

Runs, jobs, and leases follow explicit state machines defined in `docs/architecture/state-machines.md`. Illegal transitions are bugs. The orchestrator is the sole authority for state transitions.

### Diff-Aware Planning

`planner/diff_planner.go` analyzes changed files, maps them to affected projects (supports Go workspaces and monorepos), and generates job plans. Conservative fallback: run everything if uncertain.

### AI Integration

AI is advisory only — never authoritative, never applies patches automatically. Failure explanations use sanitized log inputs via the ai-proxy. See `docs/design/ai-usage.md`.

### Web Dashboard

`web/` provides a server-rendered UI mounted on `/` (non-API routes). Controlled by `--web-enabled` (default true). Uses htmx for partial updates (filtering, polling, lazy-load) with no JS build step. Templates auto-escape via `html/template`. CSP headers enforced. CSRF via double-submit cookie on POST routes.

## Key Design Rules

- ADRs in `docs/adr/` are authoritative; do not contradict them without a new ADR.
- If behavior is not documented, treat it as undefined.
- Artifacts, logs, and runner output are untrusted/hostile input — sanitize before analysis.
- Lease IDs are secrets — never log them.
- Secrets never flow to fork PRs.
- Cache keys must be deterministic and never include secrets.
- Any behavior change requires documentation updates; design shifts require new ADRs.
- Keep plans explainable: be able to state why each job runs or is skipped.

## Stack

- Go 1.25+, PostgreSQL 16+
- `jackc/pgx/v5` for database, `aws-sdk-go-v2` for S3 artifacts
- Prometheus metrics, structured logging via `log/slog`, optional OpenTelemetry
- GitHub integration (webhooks, status checks)
- Migrations run automatically on startup via `state.Migrate(ctx, db)`

## Documentation

Extensive docs live in `docs/`. Start with `docs/README.md`. Key paths:
- Architecture: `docs/architecture/`
- Design rationale: `docs/design/`
- Operations: `docs/operations/`
- API/protocol reference: `docs/reference/`
- ADRs: `docs/adr/`
- Phase checklists: `checklists/`
