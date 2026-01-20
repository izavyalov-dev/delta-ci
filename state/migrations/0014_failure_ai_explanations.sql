CREATE TABLE job_failure_ai_explanations (
    id BIGSERIAL PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    model TEXT,
    prompt_version TEXT NOT NULL,
    summary TEXT NOT NULL,
    details TEXT,
    latency_ms INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX job_failure_ai_explanations_attempt_idx ON job_failure_ai_explanations(job_attempt_id);
