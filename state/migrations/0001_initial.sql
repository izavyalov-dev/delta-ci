-- Schema migrations table
CREATE TABLE IF NOT EXISTS schema_migrations (
    id TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Runs
CREATE TABLE runs (
    id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    ref TEXT NOT NULL,
    commit_sha TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'CREATED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE runs
    ADD CONSTRAINT runs_state_check CHECK (
        state IN (
            'CREATED',
            'PLANNING',
            'PLAN_FAILED',
            'QUEUED',
            'RUNNING',
            'CANCEL_REQUESTED',
            'SUCCESS',
            'FAILED',
            'CANCELED',
            'REPORTED',
            'TIMEOUT'
        )
    );

-- Jobs
CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT TRUE,
    state TEXT NOT NULL DEFAULT 'CREATED',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE jobs
    ADD CONSTRAINT jobs_state_check CHECK (
        state IN (
            'CREATED',
            'QUEUED',
            'LEASED',
            'STARTING',
            'RUNNING',
            'UPLOADING',
            'SUCCEEDED',
            'FAILED',
            'CANCEL_REQUESTED',
            'CANCELED',
            'TIMED_OUT',
            'STALE'
        )
    );

CREATE INDEX jobs_run_id_idx ON jobs(run_id);

-- Job Attempts
CREATE TABLE job_attempts (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL,
    state TEXT NOT NULL DEFAULT 'CREATED',
    lease_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT job_attempts_attempt_number_check CHECK (attempt_number > 0)
);

ALTER TABLE job_attempts
    ADD CONSTRAINT job_attempts_state_check CHECK (
        state IN (
            'CREATED',
            'QUEUED',
            'LEASED',
            'STARTING',
            'RUNNING',
            'UPLOADING',
            'SUCCEEDED',
            'FAILED',
            'CANCEL_REQUESTED',
            'CANCELED',
            'TIMED_OUT',
            'STALE'
        )
    );

CREATE UNIQUE INDEX job_attempts_job_id_attempt_number_idx ON job_attempts(job_id, attempt_number);
CREATE INDEX job_attempts_job_id_idx ON job_attempts(job_id);

-- Leases
CREATE TABLE leases (
    id TEXT PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    runner_id TEXT,
    state TEXT NOT NULL DEFAULT 'GRANTED',
    ttl_seconds INTEGER NOT NULL CHECK (ttl_seconds > 0),
    heartbeat_interval_seconds INTEGER NOT NULL CHECK (heartbeat_interval_seconds > 0),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    last_heartbeat_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

ALTER TABLE leases
    ADD CONSTRAINT leases_state_check CHECK (
        state IN (
            'GRANTED',
            'ACTIVE',
            'EXPIRED',
            'REVOKED',
            'COMPLETED',
            'CANCELED'
        )
    );

ALTER TABLE leases
    ADD CONSTRAINT leases_ttl_heartbeat_check CHECK (ttl_seconds > heartbeat_interval_seconds);

CREATE INDEX leases_job_attempt_id_idx ON leases(job_attempt_id);
CREATE UNIQUE INDEX leases_active_per_attempt_idx ON leases(job_attempt_id) WHERE state IN ('GRANTED', 'ACTIVE');
