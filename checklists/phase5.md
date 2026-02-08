# Phase 5: Web Dashboard & Management UI

## Foundation (5.1)
- [x] Create `web/` package with `embed.FS`
- [x] Static assets: Pico CSS (classless), htmx
- [x] Base layout template with nav, footer, htmx config
- [x] CSP middleware (`script-src 'nonce-{random}'`, `style-src 'self'`, `frame-ancestors 'none'`)
- [x] Wire into `cmd/orchestrator/main.go` with `--web-enabled` / `--web-dev` flags
- [x] Static file serving via embedded FS

## Runs List (5.2)
- [x] `state.ListRuns(ctx, RunListFilter)` — paginated, filterable
- [x] `state.CountRunsByState(ctx)` — aggregate stats
- [x] `state.ListDistinctRepos(ctx)` — for filter dropdowns
- [x] Database migration `0016_list_indexes.sql`
- [x] `orchestrator.ListRuns` / `RunSummary` type
- [x] Runs page with filter sidebar, pagination
- [x] htmx partial swap on filter change

## Run Detail (5.3)
- [x] Run detail page with jobs, attempts, artifacts
- [x] Lazy-loading job expansion via htmx (`hx-trigger="click once"`)
- [x] Polling for active runs (`hx-trigger="every 5s [!document.hidden]"`)
- [x] Plan explanation and skipped jobs display

## AI Explanations & Fix Suggestions (5.4)
- [x] Failure explanations with category and confidence
- [x] AI explanations with advisory styling (`ai-advisory` class + banner)
- [x] Fix suggestion diff rendering (CSS classes for add/del/hunk)
- [x] Validation status badges

## Actions (5.5)
- [x] CSRF double-submit cookie pattern
- [x] Cancel run action (`POST /runs/{id}/cancel`)
- [x] Rerun action (`POST /runs/{id}/rerun`)
- [x] Flash messages for action feedback
- [x] `hx-confirm` for cancel action

## Dashboard (5.6)
- [x] `orchestrator.GetSystemStats(ctx)` — run counts by state
- [x] Dashboard page with stat cards
- [x] Recent runs and recent failures tables
- [x] Auto-refresh stats via htmx polling (`every 30s`)

## Settings (5.7)
- [x] Read-only settings page
- [x] Links to /healthz and /metrics

## Verification
- [x] `go build ./cmd/...` — single binary builds
- [x] `go vet ./...` — clean
- [x] Existing tests unaffected
- [x] Lease IDs stripped from web views (sanitizeRunDetailsForWeb)
- [x] CSP headers set on all web responses
- [x] Artifact URIs validated for https:// or s3:// only
- [x] AI content rendered with advisory banner
