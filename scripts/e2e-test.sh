#!/usr/bin/env bash
#
# End-to-end test: starts orchestrator + worker, sends a simulated webhook,
# and polls until the run reaches a terminal state.
#
# Prerequisites:
#   - PostgreSQL running (docker compose up -d)
#   - No other orchestrator on :8080
#
# Usage:
#   ./scripts/e2e-test.sh
#   DATABASE_URL=... WEBHOOK_SECRET=test ./scripts/e2e-test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

DATABASE_URL="${DATABASE_URL:-postgres://delta:delta@localhost:5432/delta_ci?sslmode=disable}"
WEBHOOK_SECRET="${WEBHOOK_SECRET:-e2e-test-secret}"
LISTEN="${LISTEN:-:8080}"
ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
POLL_TIMEOUT=180  # seconds
POLL_INTERVAL=3   # seconds
REPO="${E2E_REPO:-e2e-test/delta-ci}"
REF="${E2E_REF:-refs/heads/main}"
SHA="${E2E_SHA:-$(openssl rand -hex 20)}"

PIDS=()

cleanup() {
  echo ""
  echo "Cleaning up..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  done
}
trap cleanup EXIT

echo "=== Building orchestrator ==="
(cd "$PROJECT_DIR" && go build -o bin/orchestrator ./cmd/orchestrator)

echo "=== Starting orchestrator serve ==="
DATABASE_URL="$DATABASE_URL" "$PROJECT_DIR/bin/orchestrator" serve \
  -listen "$LISTEN" \
  -github-webhook-secret "$WEBHOOK_SECRET" &
PIDS+=($!)

echo "=== Starting orchestrator worker ==="
DATABASE_URL="$DATABASE_URL" "$PROJECT_DIR/bin/orchestrator" worker \
  -runner-cmd "$PROJECT_DIR/bin/runner" \
  -continue-on-runner-error=true &
PIDS+=($!)

echo "=== Waiting for healthz ==="
for i in $(seq 1 30); do
  if curl -sf "$ORCHESTRATOR_URL/healthz" >/dev/null 2>&1; then
    echo "Orchestrator healthy."
    break
  fi
  if [[ $i -eq 30 ]]; then
    echo "ERROR: orchestrator did not become healthy within 30s" >&2
    exit 1
  fi
  sleep 1
done

echo "=== Sending simulated push webhook ==="
"$SCRIPT_DIR/simulate-webhook.sh" \
  --repo "$REPO" \
  --ref "$REF" \
  --sha "$SHA" \
  --secret "$WEBHOOK_SECRET" \
  --url "$ORCHESTRATOR_URL"

echo ""
echo "=== Polling for run completion ==="
ELAPSED=0
while [[ $ELAPSED -lt $POLL_TIMEOUT ]]; do
  RESPONSE=$(curl -sf "$ORCHESTRATOR_URL/api/v1/runs" 2>/dev/null || echo "")
  if [[ -n "$RESPONSE" ]]; then
    # Look for the run matching our SHA
    STATE=$(echo "$RESPONSE" | grep -o '"state":"[^"]*"' | head -1 | cut -d'"' -f4 || true)
    if [[ -n "$STATE" ]]; then
      echo "  Run state: $STATE (${ELAPSED}s elapsed)"
      case "$STATE" in
        success|reported)
          echo ""
          echo "=== E2E TEST PASSED ==="
          echo "Run reached $STATE in ${ELAPSED}s."
          exit 0
          ;;
        failed|canceled|timeout)
          echo ""
          echo "=== E2E TEST FAILED ==="
          echo "Run reached terminal state: $STATE" >&2
          exit 1
          ;;
      esac
    fi
  fi
  sleep "$POLL_INTERVAL"
  ELAPSED=$((ELAPSED + POLL_INTERVAL))
done

echo ""
echo "=== E2E TEST FAILED ==="
echo "Timed out after ${POLL_TIMEOUT}s waiting for run to complete." >&2
exit 1
