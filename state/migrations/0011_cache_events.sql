CREATE TABLE job_cache_events (
    id BIGSERIAL PRIMARY KEY,
    job_attempt_id TEXT NOT NULL REFERENCES job_attempts(id) ON DELETE CASCADE,
    cache_type TEXT NOT NULL,
    cache_key TEXT NOT NULL,
    cache_hit BOOLEAN NOT NULL,
    read_only BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX job_cache_events_attempt_id_idx ON job_cache_events(job_attempt_id);
