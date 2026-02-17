package planner

import "context"

// LanguagePlugin detects, discovers, and plans jobs for a single language ecosystem.
type LanguagePlugin interface {
	// Name returns a short identifier (e.g. "go", "rust", "node").
	Name() string

	// Detect returns true if the repository at repoRoot uses this language.
	// It should be a fast check (stat a few marker files, not a full walk).
	Detect(ctx context.Context, repoRoot string) (bool, error)

	// Discover performs full project discovery: finds projects, their
	// dependencies, and reports which file extensions / global files matter.
	Discover(ctx context.Context, repoRoot string) (DiscoverResult, error)

	// Plan produces language-specific jobs for the impacted projects.
	Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error)
}

// DiscoverResult holds the output of a plugin's Discover step.
type DiscoverResult struct {
	Projects          []project
	DependencyUnknown bool
	CodeExtensions    []string // e.g. [".go", ".mod", ".sum"]
	GlobalFiles       []string // e.g. ["go.mod", "go.sum", "go.work"]
	FingerprintFiles  []string // files used for repo fingerprinting
}

// PluginPlanRequest is the input to a plugin's Plan step.
type PluginPlanRequest struct {
	RepoRoot      string
	Impact        impactSummary
	Explain       string
	Projects      []project
	CacheReadOnly bool
}

// PluginPlanResult is the output of a plugin's Plan step.
type PluginPlanResult struct {
	Jobs        []PlannedJob
	SkippedJobs []SkippedJob
}

// PluginRegistry holds the set of registered language plugins.
type PluginRegistry struct {
	plugins []LanguagePlugin
}

// NewPluginRegistry creates a registry with the given plugins.
func NewPluginRegistry(plugins ...LanguagePlugin) *PluginRegistry {
	return &PluginRegistry{plugins: plugins}
}

// All returns all registered plugins.
func (r *PluginRegistry) All() []LanguagePlugin {
	if r == nil {
		return nil
	}
	return r.plugins
}

// DetectAll returns plugins that detect their language in the given repo.
func (r *PluginRegistry) DetectAll(ctx context.Context, repoRoot string) []LanguagePlugin {
	if r == nil {
		return nil
	}
	var detected []LanguagePlugin
	for _, p := range r.plugins {
		ok, err := p.Detect(ctx, repoRoot)
		if err != nil {
			// Detection failure is treated as detected (conservative).
			detected = append(detected, p)
			continue
		}
		if ok {
			detected = append(detected, p)
		}
	}
	return detected
}
