package planner

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

// NodeLanguagePlugin is the built-in plugin for Node.js/JavaScript/TypeScript projects.
type NodeLanguagePlugin struct{}

func (NodeLanguagePlugin) Name() string { return "node" }

func (NodeLanguagePlugin) Detect(ctx context.Context, repoRoot string) (bool, error) {
	markers := []string{"package.json", ".nvmrc", ".node-version"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(repoRoot, m)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (NodeLanguagePlugin) Discover(ctx context.Context, repoRoot string) (DiscoverResult, error) {
	var projects []project
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || (len(name) > 0 && name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, filepath.Dir(path))
		if err != nil {
			return nil
		}
		name := rel
		if name == "." {
			name = "root"
		}
		projects = append(projects, project{
			Name:     name,
			Root:     rel,
			Language: "node",
		})
		return nil
	})
	return DiscoverResult{
		Projects:          projects,
		CodeExtensions:    []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs"},
		GlobalFiles:       []string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml"},
		FingerprintFiles:  []string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml"},
	}, nil
}

func (NodeLanguagePlugin) Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error) {
	targetProjects := req.Impact.ImpactedProjects
	if len(targetProjects) == 0 {
		targetProjects = projectNames(req.Projects)
	}
	if len(targetProjects) == 0 {
		return PluginPlanResult{}, nil
	}
	sort.Strings(targetProjects)

	projectRoots := buildProjectRootIndex(req.Projects)
	var jobs []PlannedJob
	for _, name := range targetProjects {
		root := projectRoots[name]
		if root == "" {
			root = "."
		}
		buildName := jobNameForProject("build", name, root)
		jobs = append(jobs, PlannedJob{
			Name:     buildName,
			Required: true,
			Spec: protocol.JobSpec{
				Name:    buildName,
				Workdir: root,
				Steps:   []string{"npm install && npm run build --if-present"},
			},
			Reason: req.Explain,
		})
		if !req.Impact.DocsOnly {
			testName := jobNameForProject("test", name, root)
			jobs = append(jobs, PlannedJob{
				Name:     testName,
				Required: true,
				Spec: protocol.JobSpec{
					Name:    testName,
					Workdir: root,
					Steps:   []string{"npm run test --if-present"},
				},
				Reason:    req.Explain,
				DependsOn: []string{buildName},
			})
			lintName := jobNameForProject("lint", name, root)
			jobs = append(jobs, PlannedJob{
				Name:     lintName,
				Required: false,
				Spec: protocol.JobSpec{
					Name:    lintName,
					Workdir: root,
					Steps:   []string{"npm run lint --if-present"},
				},
				Reason:    req.Explain,
				DependsOn: []string{buildName},
			})
		}
	}
	return PluginPlanResult{Jobs: jobs}, nil
}
