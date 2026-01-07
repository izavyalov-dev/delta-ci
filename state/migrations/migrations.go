package migrations

import _ "embed"

// Migration represents a single SQL migration to apply in order.
type Migration struct {
	ID     string
	Script string
}

//go:embed 0001_initial.sql
var initial string

//go:embed 0002_job_queue.sql
var queue string

//go:embed 0003_artifacts.sql
var artifacts string

// All lists migrations in application order.
var All = []Migration{
	{ID: "0001_initial", Script: initial},
	{ID: "0002_job_queue", Script: queue},
	{ID: "0003_artifacts", Script: artifacts},
}
