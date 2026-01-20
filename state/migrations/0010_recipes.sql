CREATE TABLE recipes (
    id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    fingerprint TEXT NOT NULL,
    version INTEGER NOT NULL,
    source TEXT NOT NULL,
    recipe_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX recipes_repo_fingerprint_version_idx ON recipes(repo_id, fingerprint, version);
CREATE INDEX recipes_repo_fingerprint_idx ON recipes(repo_id, fingerprint);

CREATE TABLE run_plans (
    run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
    repo_id TEXT NOT NULL,
    fingerprint TEXT,
    recipe_id TEXT REFERENCES recipes(id),
    recipe_source TEXT NOT NULL,
    recipe_version INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX run_plans_repo_id_idx ON run_plans(repo_id);
CREATE INDEX run_plans_recipe_id_idx ON run_plans(recipe_id);
