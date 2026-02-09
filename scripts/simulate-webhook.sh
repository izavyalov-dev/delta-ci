#!/usr/bin/env bash
#
# Sends a simulated GitHub push or pull_request webhook to local orchestrator.
#
# Usage:
#   ./scripts/simulate-webhook.sh --repo owner/repo --ref refs/heads/main --sha abc123 --secret mysecret
#   ./scripts/simulate-webhook.sh --repo owner/repo --ref refs/heads/main --sha abc123 --event-type pull_request --pr-number 42 --secret mysecret

set -euo pipefail

ORCHESTRATOR_URL="${ORCHESTRATOR_URL:-http://localhost:8080}"
EVENT_TYPE="push"
REPO=""
REF=""
SHA=""
SECRET=""
PR_NUMBER=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)       REPO="$2"; shift 2 ;;
    --ref)        REF="$2"; shift 2 ;;
    --sha)        SHA="$2"; shift 2 ;;
    --secret)     SECRET="$2"; shift 2 ;;
    --event-type) EVENT_TYPE="$2"; shift 2 ;;
    --pr-number)  PR_NUMBER="$2"; shift 2 ;;
    --url)        ORCHESTRATOR_URL="$2"; shift 2 ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$REPO" || -z "$REF" || -z "$SHA" ]]; then
  echo "Usage: $0 --repo owner/repo --ref refs/heads/main --sha <commit-sha> [--secret <secret>] [--event-type push|pull_request] [--pr-number N]" >&2
  exit 1
fi

OWNER="${REPO%%/*}"
REPO_NAME="${REPO##*/}"

if [[ "$EVENT_TYPE" == "push" ]]; then
  PAYLOAD=$(cat <<EOJSON
{
  "ref": "$REF",
  "after": "$SHA",
  "repository": {
    "full_name": "$REPO",
    "owner": { "login": "$OWNER" },
    "name": "$REPO_NAME",
    "clone_url": "https://github.com/$REPO.git",
    "default_branch": "main"
  },
  "head_commit": {
    "id": "$SHA",
    "message": "simulated push"
  },
  "sender": { "login": "simulate-webhook" }
}
EOJSON
)
elif [[ "$EVENT_TYPE" == "pull_request" ]]; then
  PAYLOAD=$(cat <<EOJSON
{
  "action": "synchronize",
  "number": ${PR_NUMBER:-1},
  "pull_request": {
    "number": ${PR_NUMBER:-1},
    "head": {
      "sha": "$SHA",
      "ref": "$REF"
    },
    "base": {
      "ref": "main"
    }
  },
  "repository": {
    "full_name": "$REPO",
    "owner": { "login": "$OWNER" },
    "name": "$REPO_NAME",
    "clone_url": "https://github.com/$REPO.git",
    "default_branch": "main"
  },
  "sender": { "login": "simulate-webhook" }
}
EOJSON
)
else
  echo "Unsupported event type: $EVENT_TYPE" >&2
  exit 1
fi

CURL_ARGS=(
  -s -w "\nHTTP %{http_code}\n"
  -X POST
  -H "Content-Type: application/json"
  -H "X-GitHub-Event: $EVENT_TYPE"
  -H "X-GitHub-Delivery: $(uuidgen 2>/dev/null || cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "00000000-0000-0000-0000-000000000000")"
)

if [[ -n "$SECRET" ]]; then
  SIGNATURE=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $NF}')
  CURL_ARGS+=(-H "X-Hub-Signature-256: sha256=$SIGNATURE")
fi

CURL_ARGS+=(-d "$PAYLOAD")

echo "Sending $EVENT_TYPE webhook to $ORCHESTRATOR_URL/api/v1/webhooks/github ..."
curl "${CURL_ARGS[@]}" "$ORCHESTRATOR_URL/api/v1/webhooks/github"
