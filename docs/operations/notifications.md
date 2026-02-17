# Notifications

Delta CI can notify external systems when run states change. Notifications are configured via CLI flags or environment variables on the orchestrator.

## Reporters

### GitHub Status Checks (built-in)

GitHub status checks are configured via `--github-token` or GitHub App credentials. See [github-app-setup.md](github-app-setup.md).

### Generic Webhook

Posts a JSON payload to any URL on run state changes. Use this to integrate with PagerDuty, email (via Zapier/n8n), or custom systems.

**Flags:**

| Flag | Env | Description |
|------|-----|-------------|
| `--notify-webhook-url` | `DELTA_NOTIFY_WEBHOOK_URL` | Target URL (required to enable) |
| `--notify-webhook-secret` | `DELTA_NOTIFY_WEBHOOK_SECRET` | HMAC-SHA256 signing secret |
| `--notify-webhook-events` | `DELTA_NOTIFY_WEBHOOK_EVENTS` | `all` or `terminal-only` (default) |

**Payload:**

```json
{
  "event": "run.state_changed",
  "run_id": "abc123",
  "repo_id": "my-org/my-repo",
  "ref": "refs/heads/main",
  "commit_sha": "deadbeef",
  "state": "SUCCESS",
  "jobs": [
    {"name": "test", "state": "SUCCESS", "required": true},
    {"name": "lint", "state": "SUCCESS", "required": false}
  ],
  "duration_ms": 45200,
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:00:45Z",
  "dashboard_url": "https://ci.example.com/runs/abc123"
}
```

**Signature verification:**

When `--notify-webhook-secret` is set, requests include an `X-DeltaCI-Signature` header with `sha256=<hex>`. Verify using HMAC-SHA256 of the raw request body with the shared secret.

### Slack

Sends Block Kit messages via Slack incoming webhooks. No bot token required.

**Flags:**

| Flag | Env | Description |
|------|-----|-------------|
| `--notify-slack-webhook-url` | `DELTA_NOTIFY_SLACK_WEBHOOK_URL` | Slack webhook URL (required to enable) |
| `--notify-slack-events` | `DELTA_NOTIFY_SLACK_EVENTS` | `all` or `terminal-only` (default) |

**Setup:**

1. In Slack, go to **Apps > Manage > Custom Integrations > Incoming Webhooks** (or create a Slack App with an incoming webhook).
2. Choose a channel and copy the webhook URL.
3. Pass the URL via `--notify-slack-webhook-url` or `DELTA_NOTIFY_SLACK_WEBHOOK_URL`.

Messages include run state, repo, ref, commit SHA, duration, per-job status, and a link to the dashboard (if `--notify-dashboard-url` is set).

## Common Options

| Flag | Env | Description |
|------|-----|-------------|
| `--notify-dashboard-url` | `DELTA_DASHBOARD_URL` | Base URL for dashboard links in notifications |

## Event Filters

- `terminal-only` (default): Only notify on terminal states — SUCCESS, FAILED, CANCELED, TIMEOUT, REPORTED.
- `all`: Notify on every state change.

## Multi-Reporter

All configured reporters fire in parallel. If one fails, the others still execute. Errors from all reporters are collected and logged.
