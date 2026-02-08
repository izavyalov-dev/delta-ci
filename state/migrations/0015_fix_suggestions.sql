CREATE TABLE job_fix_suggestions (
    id BIGSERIAL PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT,
    prompt_version TEXT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    patch_format TEXT NOT NULL,
    patch_unified_diff TEXT NOT NULL,
    patch_sha256 TEXT NOT NULL,
    validation_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
    validation_job_id TEXT REFERENCES jobs(id) ON DELETE SET NULL,
    validation_status TEXT NOT NULL DEFAULT 'PENDING',
    validation_summary TEXT,
    requires_approval BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_attempt_id, patch_sha256)
);

CREATE INDEX job_fix_suggestions_job_attempt_idx ON job_fix_suggestions(job_attempt_id);
CREATE INDEX job_fix_suggestions_validation_job_idx ON job_fix_suggestions(validation_job_id);
