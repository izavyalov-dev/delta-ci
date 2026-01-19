CREATE TABLE job_dependencies (
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    depends_on_job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (job_id, depends_on_job_id),
    CONSTRAINT job_dependencies_no_self CHECK (job_id <> depends_on_job_id)
);

CREATE INDEX job_dependencies_job_id_idx ON job_dependencies(job_id);
CREATE INDEX job_dependencies_depends_on_idx ON job_dependencies(depends_on_job_id);
