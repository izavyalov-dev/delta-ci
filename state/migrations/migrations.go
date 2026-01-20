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

//go:embed 0004_job_specs.sql
var jobSpecs string

//go:embed 0005_run_triggers.sql
var runTriggers string

//go:embed 0006_status_reports.sql
var statusReports string

//go:embed 0007_run_reruns.sql
var runReruns string

//go:embed 0008_failure_explanations.sql
var failureExplanations string

//go:embed 0009_job_dependencies.sql
var jobDependencies string

//go:embed 0010_recipes.sql
var recipes string

//go:embed 0011_cache_events.sql
var cacheEvents string

//go:embed 0012_explainability.sql
var explainability string

//go:embed 0013_failure_explanations_signals.sql
var failureExplanationSignals string

//go:embed 0014_failure_ai_explanations.sql
var failureAIExplanations string

// All lists migrations in application order.
var All = []Migration{
	{ID: "0001_initial", Script: initial},
	{ID: "0002_job_queue", Script: queue},
	{ID: "0003_artifacts", Script: artifacts},
	{ID: "0004_job_specs", Script: jobSpecs},
	{ID: "0005_run_triggers", Script: runTriggers},
	{ID: "0006_status_reports", Script: statusReports},
	{ID: "0007_run_reruns", Script: runReruns},
	{ID: "0008_failure_explanations", Script: failureExplanations},
	{ID: "0009_job_dependencies", Script: jobDependencies},
	{ID: "0010_recipes", Script: recipes},
	{ID: "0011_cache_events", Script: cacheEvents},
	{ID: "0012_explainability", Script: explainability},
	{ID: "0013_failure_explanations_signals", Script: failureExplanationSignals},
	{ID: "0014_failure_ai_explanations", Script: failureAIExplanations},
}
