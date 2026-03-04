package planner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

// DotNetLanguagePlugin is the built-in plugin for .NET (C#/F#/VB) projects.
type DotNetLanguagePlugin struct{}

func (DotNetLanguagePlugin) Name() string { return "dotnet" }

func (DotNetLanguagePlugin) Detect(ctx context.Context, repoRoot string) (bool, error) {
	return dotnetHasProjectFiles(repoRoot), nil
}

// dotnetHasProjectFiles returns true if the directory tree contains any .NET project files.
func dotnetHasProjectFiles(repoRoot string) bool {
	found := false
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "bin" || name == "obj" || (len(name) > 0 && name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		if isDotNetProjectFile(d.Name()) {
			found = true
		}
		return nil
	})
	return found
}

func isDotNetProjectFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".csproj") ||
		strings.HasSuffix(lower, ".fsproj") ||
		strings.HasSuffix(lower, ".vbproj") ||
		strings.HasSuffix(lower, ".sln")
}

func (DotNetLanguagePlugin) Discover(ctx context.Context, repoRoot string) (DiscoverResult, error) {
	// Walk for solution files first; fall back to individual project files.
	var slnFiles []string
	var projFiles []string

	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "bin" || name == "obj" || (len(name) > 0 && name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(d.Name())
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		if strings.HasSuffix(lower, ".sln") {
			slnFiles = append(slnFiles, rel)
		} else if strings.HasSuffix(lower, ".csproj") || strings.HasSuffix(lower, ".fsproj") || strings.HasSuffix(lower, ".vbproj") {
			projFiles = append(projFiles, rel)
		}
		return nil
	})

	var projects []project
	if len(slnFiles) > 0 {
		// One project per solution file.
		for _, sln := range slnFiles {
			dir := filepath.Dir(sln)
			name := strings.TrimSuffix(filepath.Base(sln), filepath.Ext(sln))
			if dir == "." {
				name = "root"
			}
			projects = append(projects, project{
				Name:     name,
				Root:     dir,
				Language: "dotnet",
			})
		}
	} else {
		// No solution file: treat root as a single project.
		projects = append(projects, project{
			Name:     "root",
			Root:     ".",
			Language: "dotnet",
		})
	}

	// Build fingerprint from project files.
	fpFiles := make([]string, 0, len(slnFiles)+len(projFiles)+1)
	fpFiles = append(fpFiles, slnFiles...)
	fpFiles = append(fpFiles, projFiles...)
	if _, err := os.Stat(filepath.Join(repoRoot, "Directory.Build.props")); err == nil {
		fpFiles = append(fpFiles, "Directory.Build.props")
	}

	return DiscoverResult{
		Projects:         projects,
		CodeExtensions:   []string{".cs", ".fs", ".vb", ".razor", ".cshtml"},
		GlobalFiles:      slnFiles,
		FingerprintFiles: fpFiles,
	}, nil
}

func (DotNetLanguagePlugin) Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error) {
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
				Steps:   []string{"dotnet build"},
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
					Steps:   []string{"dotnet test"},
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
					Steps:   []string{"dotnet format --verify-no-changes 2>/dev/null || true"},
				},
				Reason:    req.Explain,
				DependsOn: []string{buildName},
			})
		}
	}
	return PluginPlanResult{Jobs: jobs}, nil
}
