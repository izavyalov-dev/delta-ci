-- Failure explanations for job attempts
CREATE TABLE job_failure_explanations (
    id BIGSERIAL PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    category TEXT NOT NULL,
    summary TEXT NOT NULL,
    confidence TEXT NOT NULL DEFAULT 'LOW',
    details TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE job_failure_explanations
    ADD CONSTRAINT job_failure_explanations_category_check CHECK (
        category IN ('USER', 'INFRA', 'TOOLING', 'FLAKY', 'CANCELED', 'UNKNOWN')
    );

ALTER TABLE job_failure_explanations
    ADD CONSTRAINT job_failure_explanations_confidence_check CHECK (
        confidence IN ('LOW', 'MEDIUM', 'HIGH')
    );

CREATE UNIQUE INDEX job_failure_explanations_attempt_idx ON job_failure_explanations(job_attempt_id);
