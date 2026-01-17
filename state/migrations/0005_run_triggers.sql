-- Run triggers for webhook idempotency and VCS metadata
CREATE TABLE run_triggers (
    run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    event_key TEXT NOT NULL,
    event_type TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    repo_owner TEXT NOT NULL,
    repo_name TEXT NOT NULL,
    pr_number INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT run_triggers_pr_number_check CHECK (pr_number IS NULL OR pr_number > 0)
);

CREATE UNIQUE INDEX run_triggers_provider_event_key_idx ON run_triggers(provider, event_key);
CREATE INDEX run_triggers_provider_repo_pr_idx ON run_triggers(provider, repo_owner, repo_name, pr_number);
