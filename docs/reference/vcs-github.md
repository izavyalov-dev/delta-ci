# GitHub Integration

This document describes the **GitHub-specific integration** used in Phase 1.

It covers:
- webhook ingestion
- idempotency behavior
- check runs and PR comments
- required configuration

If behavior is not documented here, it is undefined.

---

## Configuration

Delta CI reads GitHub configuration from flags or environment variables.

Required for webhooks:
- `-github-webhook-secret` or `GITHUB_WEBHOOK_SECRET`

Required for status reporting:
- `-github-token` or `GITHUB_TOKEN`, or GitHub App credentials (see below)

Optional:
- `-github-api-url` or `GITHUB_API_URL` (default: `https://api.github.com`)
- `-github-check-name` or `GITHUB_CHECK_NAME` (default: `delta-ci`)
- GitHub App auth (preferred when checks API requires it):
  - `-github-app-id` or `GITHUB_APP_ID`
  - `-github-app-installation-id` or `GITHUB_APP_INSTALLATION_ID`
  - `-github-app-private-key-file` or `GITHUB_APP_PRIVATE_KEY_FILE` 
  - `-github-app-private-key` or `GITHUB_APP_PRIVATE_KEY` (PEM, supports `\n`)

---

## Webhook Setup (GitHub UI)

Recommended settings in GitHub:
- Payload URL: `https://<public-host>/api/v1/webhooks/github`
- Content type: `application/json`
- Secret: match `GITHUB_WEBHOOK_SECRET`
- Events: `push` and `pull_request`

For local development, forward a public URL to your local orchestrator with a relay
service (for example, smee) and keep the same endpoint path.

---

## Webhook Endpoint

```
POST /api/v1/webhooks/github
```

Required headers:
- `X-GitHub-Event`
- `X-Hub-Signature-256` (preferred) or `X-Hub-Signature`

Payload size limit:
- 1 MiB (requests above the limit are rejected)

### Supported Events

Delta CI creates runs for these events only:

1) `push`
   - uses `ref` and `after` (commit SHA)
   - deleted refs are ignored

2) `pull_request`
   - supported actions: `opened`, `synchronize`, `reopened`
   - uses `pull_request.head.sha`
   - ref is normalized to `refs/pull/<number>/head`

Other events are accepted but ignored.

---

## Idempotency

Webhook idempotency key is derived from:
- repository ID (`owner/name`)
- commit SHA
- event type (`push` or `pull_request`)
- PR number (when present)

Duplicate deliveries must not create duplicate runs.

---

## Status Reporting

### Check Runs

Check run status reflects orchestrator state:

- `CREATED`, `PLANNING`, `QUEUED` → `queued`
- `RUNNING` → `in_progress`
- `SUCCESS` → `completed` / `success`
- `FAILED` or `PLAN_FAILED` → `completed` / `failure`
- `CANCELED` → `completed` / `cancelled`
- `TIMEOUT` → `completed` / `timed_out`

### PR Comments

PR comments are posted or updated **only on terminal states**:
- `SUCCESS`
- `FAILED`
- `CANCELED`
- `TIMEOUT`
- `PLAN_FAILED`

Comments are updated in-place for the same run.

### Authentication Notes

Some GitHub orgs require **GitHub App** authentication to create check runs.
If you see errors like `"You must authenticate via a GitHub App"`, configure
the app credentials listed above and restart the orchestrator.

---

## Security Notes

- Webhook signatures are always verified.
- GitHub tokens are required for status reporting.
- Secrets never flow to fork PRs (see `architecture/security-model.md`).

---

## Related Documents

- `reference/api-contracts.md`
- `architecture/control-plane.md`
- `architecture/security-model.md`
