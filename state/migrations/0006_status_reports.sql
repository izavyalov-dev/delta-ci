-- VCS status reporting metadata (check runs + PR comments)
CREATE TABLE vcs_status_reports (
    run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    check_run_id TEXT,
    pr_comment_id TEXT,
    last_state TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, provider)
);
