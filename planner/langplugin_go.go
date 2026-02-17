package planner

import (
	"context"
	"os"
	"path/filepath"
)

// GoLanguagePlugin is the built-in plugin for Go projects.
type GoLanguagePlugin struct{}

func (GoLanguagePlugin) Name() string { return "go" }

func (GoLanguagePlugin) Detect(ctx context.Context, repoRoot string) (bool, error) {
	markers := []string{"go.mod", "go.sum", "go.work", "go.work.sum"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(repoRoot, m)); err == nil {
			return true, nil
		}
	}
	// Also check if discoverGoProjects finds anything (nested go.mod files).
	discovery := discoverGoProjects(repoRoot)
	return len(discovery.projects) > 0, nil
}

func (GoLanguagePlugin) Discover(ctx context.Context, repoRoot string) (DiscoverResult, error) {
	discovery := discoverGoProjects(repoRoot)
	return DiscoverResult{
		Projects:          discovery.projects,
		DependencyUnknown: discovery.dependencyUnknown,
		CodeExtensions:    []string{".go", ".mod", ".sum"},
		GlobalFiles:       []string{"go.mod", "go.sum", "go.work", "go.work.sum"},
		FingerprintFiles:  []string{"go.mod", "go.sum", "go.work", "go.work.sum"},
	}, nil
}

func (GoLanguagePlugin) Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error) {
	result := planForGo(req.Impact, req.Explain, req.Projects, req.RepoRoot, req.CacheReadOnly)
	return PluginPlanResult{
		Jobs:        result.Jobs,
		SkippedJobs: result.SkippedJobs,
	}, nil
}
