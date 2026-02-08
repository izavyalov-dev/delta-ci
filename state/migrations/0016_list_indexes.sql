CREATE INDEX IF NOT EXISTS idx_runs_repo_state_created ON runs (repo_id, state, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_created_desc ON runs (created_at DESC, id DESC);
