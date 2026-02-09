CREATE TABLE IF NOT EXISTS dead_letter_log (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::TEXT,
    attempt_id TEXT NOT NULL,
    delivery_count INTEGER NOT NULL,
    reason TEXT NOT NULL DEFAULT 'max_deliveries_exceeded',
    dead_lettered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dead_letter_log_attempt ON dead_letter_log (attempt_id);
