ALTER TABLE job_failure_explanations
    ADD COLUMN rule_version TEXT,
    ADD COLUMN signals JSONB;
