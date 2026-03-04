package planner

import (
	"context"
	"os"
	"path/filepath"
	"sort"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

// PythonLanguagePlugin is the built-in plugin for Python projects.
type PythonLanguagePlugin struct{}

func (PythonLanguagePlugin) Name() string { return "python" }

func (PythonLanguagePlugin) Detect(ctx context.Context, repoRoot string) (bool, error) {
	markers := []string{"pyproject.toml", "requirements.txt", "setup.py", "setup.cfg", "Pipfile"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(repoRoot, m)); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (PythonLanguagePlugin) Discover(ctx context.Context, repoRoot string) (DiscoverResult, error) {
	// Check which install mechanism is available at the repo root.
	installStep := pythonInstallStep(repoRoot)

	projects := []project{
		{
			Name:     "root",
			Root:     ".",
			Language: "python",
			// Store install command in ModulePath so Plan can access it.
			ModulePath: installStep,
		},
	}
	return DiscoverResult{
		Projects:         projects,
		CodeExtensions:   []string{".py", ".pyx", ".pxd"},
		GlobalFiles:      []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"},
		FingerprintFiles: []string{"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock", "poetry.lock"},
	}, nil
}

// pythonInstallStep returns a shell command that installs project dependencies.
func pythonInstallStep(root string) string {
	if _, err := os.Stat(filepath.Join(root, "requirements.txt")); err == nil {
		return "pip install -r requirements.txt"
	}
	if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
		return "pip install -e ."
	}
	if _, err := os.Stat(filepath.Join(root, "Pipfile")); err == nil {
		return "pip install -r requirements.txt"
	}
	return "pip install -e ."
}

func (PythonLanguagePlugin) Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error) {
	targetProjects := req.Impact.ImpactedProjects
	if len(targetProjects) == 0 {
		targetProjects = projectNames(req.Projects)
	}
	if len(targetProjects) == 0 {
		return PluginPlanResult{}, nil
	}
	sort.Strings(targetProjects)

	projectRoots := buildProjectRootIndex(req.Projects)
	// Index install steps stored in ModulePath.
	installSteps := make(map[string]string, len(req.Projects))
	for _, p := range req.Projects {
		installSteps[p.Name] = p.ModulePath
	}

	var jobs []PlannedJob
	for _, name := range targetProjects {
		root := projectRoots[name]
		if root == "" {
			root = "."
		}
		install := installSteps[name]
		if install == "" {
			install = "pip install -e ."
		}

		buildName := jobNameForProject("build", name, root)
		jobs = append(jobs, PlannedJob{
			Name:     buildName,
			Required: true,
			Spec: protocol.JobSpec{
				Name:    buildName,
				Workdir: root,
				Steps:   []string{install},
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
					Steps:   []string{"python -m pytest --tb=short 2>/dev/null || python -m unittest discover"},
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
					Steps:   []string{"python -m ruff check . 2>/dev/null || python -m flake8 . 2>/dev/null || true"},
				},
				Reason:    req.Explain,
				DependsOn: []string{buildName},
			})
		}
	}
	return PluginPlanResult{Jobs: jobs}, nil
}
