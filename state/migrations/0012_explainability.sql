ALTER TABLE run_plans
    ADD COLUMN explain TEXT,
    ADD COLUMN skipped_jobs JSONB;

ALTER TABLE jobs
    ADD COLUMN reason TEXT;
