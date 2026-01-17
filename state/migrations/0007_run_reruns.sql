-- Rerun idempotency mapping
CREATE TABLE run_reruns (
    original_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    new_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (original_run_id, idempotency_key)
);

CREATE UNIQUE INDEX run_reruns_new_run_id_idx ON run_reruns(new_run_id);
