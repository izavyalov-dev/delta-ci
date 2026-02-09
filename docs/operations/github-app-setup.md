# GitHub App Setup

This guide covers creating and configuring a GitHub App for Delta CI.

## 1. Create a GitHub App

Go to **Settings → Developer settings → GitHub Apps → New GitHub App**.

### App settings

| Field | Value |
|-------|-------|
| Name | `Delta CI` (or your preferred name) |
| Homepage URL | Your Delta CI instance URL |
| Webhook URL | `https://<your-host>/api/v1/webhooks/github` |
| Webhook secret | A random secret (save this for later) |

### Permissions

| Permission | Access |
|------------|--------|
| Checks | Read & Write |
| Pull requests | Read & Write |
| Contents | Read-only |

### Events to subscribe

- `push`
- `pull_request`

Click **Create GitHub App**.

## 2. Generate a Private Key

On the app settings page, scroll to **Private keys** and click **Generate a private key**. Save the `.pem` file securely.

## 3. Install the App

Go to your app's page and click **Install App**. Select the repository (or all repositories) you want Delta CI to monitor.

Note the **Installation ID** from the URL after installing: `https://github.com/settings/installations/<INSTALLATION_ID>`.

The **App ID** is shown on the app's settings page under "About".

## 4. Configure Delta CI

Pass the GitHub App credentials to the orchestrator:

```bash
./bin/orchestrator serve \
  --github-app-id 12345 \
  --github-app-installation-id 67890 \
  --github-app-private-key-file /path/to/private-key.pem \
  --github-webhook-secret "your-webhook-secret"
```

Or via environment variables:

```bash
export GITHUB_APP_ID=12345
export GITHUB_APP_INSTALLATION_ID=67890
export GITHUB_APP_PRIVATE_KEY_FILE=/path/to/private-key.pem
export GITHUB_WEBHOOK_SECRET=your-webhook-secret
```

### Optional flags

| Flag | Env var | Description |
|------|---------|-------------|
| `--github-api-url` | `GITHUB_API_URL` | Override API base URL (for GitHub Enterprise) |
| `--github-check-name` | `GITHUB_CHECK_NAME` | Custom name for the check run (default: `Delta CI`) |

## 5. Verify the Setup

Push a commit to the monitored repository and verify:

1. The webhook is received (check orchestrator logs for `webhook_received` event)
2. A run is created and jobs are planned
3. Check runs appear on the commit in GitHub

You can also use the simulate-webhook script for local testing:

```bash
./scripts/simulate-webhook.sh \
  --repo owner/repo \
  --ref refs/heads/main \
  --sha $(git rev-parse HEAD) \
  --secret "your-webhook-secret"
```

## Troubleshooting

- **No webhook received**: Verify the webhook URL is publicly reachable and the secret matches
- **401/403 from GitHub API**: Check that the private key file is correct and the installation ID is valid
- **No check runs created**: Ensure the app has `checks: write` permission on the repository
