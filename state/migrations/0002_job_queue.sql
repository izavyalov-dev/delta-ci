-- Job dispatch queue (at-least-once semantics)
CREATE TABLE job_queue (
    attempt_id TEXT PRIMARY KEY REFERENCES job_attempts(id) ON DELETE CASCADE,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    inflight_until TIMESTAMPTZ,
    delivery_count INTEGER NOT NULL DEFAULT 0,
    last_delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX job_queue_available_at_idx ON job_queue(available_at);
