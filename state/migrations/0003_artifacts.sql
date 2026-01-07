-- Artifact references (metadata only; log contents stored externally)
CREATE TABLE job_artifacts (
    id BIGSERIAL PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    artifact_type TEXT NOT NULL,
    uri TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX job_artifacts_attempt_uri_idx ON job_artifacts(job_attempt_id, uri);
CREATE INDEX job_artifacts_attempt_id_idx ON job_artifacts(job_attempt_id);
