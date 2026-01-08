# Dogfooding (Phase 0)

This guide shows how to run Delta CI against the Delta CI repository in a local
environment. It assumes a **local Postgres container** and an **AWS S3 bucket**
for log uploads.

## Prerequisites

- Go installed (1.25+)
- Docker (for PostgreSQL)
- AWS credentials with write access to the artifact bucket

AWS credentials are read via the standard AWS SDK chain:
- `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY`
- `AWS_PROFILE`
- `AWS_REGION` or runner flag `-s3-region`

## Start PostgreSQL (Docker)

```bash
docker run --name delta-ci-postgres \
  -e POSTGRES_USER=delta \
  -e POSTGRES_PASSWORD=delta \
  -e POSTGRES_DB=delta_ci \
  -p 5432:5432 \
  postgres:16
```

Set the connection string:

```bash
export DATABASE_URL="postgres://delta:delta@localhost:5432/delta_ci?sslmode=disable"
```

## Run Dogfood Build + Test

From the repo root:

```bash
go run ./cmd/orchestrator dogfood \
  -database-url "$DATABASE_URL" \
  -s3-bucket "<your-bucket>" \
  -s3-prefix "delta-ci/dogfood" \
  -s3-region "us-east-1"
```

This command:
- creates a run with `build` and `test` jobs
- persists job specs in the DB
- starts a minimal HTTP server for runner callbacks
- executes the runner locally for each job
- uploads runner logs to S3 (if configured)

Runner logs are stored under `.delta-ci/logs/` by default.

If your environment requires it, you can disable cgo:

```bash
export CGO_ENABLED=0
```

## Validation Checklist (Manual)

### Lease Expiration

1. Start the dogfood run.
2. Kill the runner process during the first job (the `go run ./runner` child).
3. Either restart the dogfood run pointing at the same database, or pass `-continue-on-runner-error`
   so the loop keeps running after the runner exits.
4. After ~2 minutes (lease TTL), observe the lease sweep re-queue the attempt and a new lease is granted.

### Runner Crash Recovery

1. Start the dogfood run.
2. Kill the runner process mid-job.
3. Restart the dogfood run (or use `-continue-on-runner-error`) and confirm the job is re-queued
   and completes after the lease expires.

### Orchestrator Restart Recovery

1. Start the dogfood run.
2. Stop the dogfood process mid-run.
3. Restart the dogfood run with the same database URL.
4. Confirm the run state, jobs, and leases are recovered from the DB after the lease sweep.

If any step fails, inspect logs and DB state before retrying.
