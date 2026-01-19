package planner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeImpactDocsOnlySingleProject(t *testing.T) {
	projects := []project{
		{
			Name:       "root",
			Root:       ".",
			Language:   "go",
			ModulePath: "example.com/root",
		},
	}

	impact := analyzeImpact([]string{"README.md"}, projects, false)
	if !impact.DocsOnly {
		t.Fatalf("expected docs-only impact")
	}
	if impact.Global {
		t.Fatalf("expected non-global impact")
	}
	if impact.CodeChanges {
		t.Fatalf("expected no code changes")
	}
	if len(impact.UnknownOwnership) != 0 {
		t.Fatalf("expected no unknown ownership, got %v", impact.UnknownOwnership)
	}
	if !reflect.DeepEqual(impact.ImpactedProjects, []string{"root"}) {
		t.Fatalf("unexpected impacted projects: %v", impact.ImpactedProjects)
	}
}

func TestAnalyzeImpactUnknownOwnershipGlobal(t *testing.T) {
	projects := []project{
		{
			Name:       "services/api",
			Root:       "services/api",
			Language:   "go",
			ModulePath: "example.com/api",
		},
		{
			Name:       "services/web",
			Root:       "services/web",
			Language:   "go",
			ModulePath: "example.com/web",
		},
	}

	impact := analyzeImpact([]string{"infra/terraform/main.tf"}, projects, false)
	if !impact.Global {
		t.Fatalf("expected global impact")
	}
	if len(impact.UnknownOwnership) != 1 {
		t.Fatalf("expected unknown ownership, got %v", impact.UnknownOwnership)
	}
	if !reflect.DeepEqual(impact.ImpactedProjects, []string{"services/api", "services/web"}) {
		t.Fatalf("unexpected impacted projects: %v", impact.ImpactedProjects)
	}
}

func TestAnalyzeImpactDependencyPropagation(t *testing.T) {
	projects := []project{
		{
			Name:       "apps/app",
			Root:       "apps/app",
			Language:   "go",
			ModulePath: "example.com/app",
			Requires:   []string{"example.com/lib"},
		},
		{
			Name:       "libs/lib",
			Root:       "libs/lib",
			Language:   "go",
			ModulePath: "example.com/lib",
		},
	}

	impact := analyzeImpact([]string{"libs/lib/lib.go"}, projects, false)
	if impact.Global {
		t.Fatalf("expected non-global impact")
	}
	if !reflect.DeepEqual(impact.ImpactedProjects, []string{"apps/app", "libs/lib"}) {
		t.Fatalf("unexpected impacted projects: %v", impact.ImpactedProjects)
	}
}

func TestAnalyzeImpactPrefersSpecificOwner(t *testing.T) {
	projects := []project{
		{
			Name:       "root",
			Root:       ".",
			Language:   "go",
			ModulePath: "example.com/root",
		},
		{
			Name:       "services/api",
			Root:       "services/api",
			Language:   "go",
			ModulePath: "example.com/api",
		},
	}

	impact := analyzeImpact([]string{"services/api/main.go"}, projects, false)
	if impact.Global {
		t.Fatalf("expected non-global impact")
	}
	if !reflect.DeepEqual(impact.ImpactedProjects, []string{"services/api"}) {
		t.Fatalf("unexpected impacted projects: %v", impact.ImpactedProjects)
	}
}

func TestPlanForGoPerProjectJobs(t *testing.T) {
	projects := []project{
		{
			Name:       "services/api",
			Root:       "services/api",
			Language:   "go",
			ModulePath: "example.com/api",
		},
		{
			Name:       "libs/lib",
			Root:       "libs/lib",
			Language:   "go",
			ModulePath: "example.com/lib",
		},
	}

	impact := analyzeImpact([]string{"services/api/main.go"}, projects, false)
	plan := planForGo(impact, "explain", projects)
	if len(plan.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(plan.Jobs))
	}

	expectedNames := []string{"build:services/api", "test:services/api", "lint:services/api"}
	for i, job := range plan.Jobs {
		if job.Name != expectedNames[i] {
			t.Fatalf("unexpected job name %q, expected %q", job.Name, expectedNames[i])
		}
		if job.Spec.Workdir != "services/api" {
			t.Fatalf("unexpected workdir %q for %s", job.Spec.Workdir, job.Name)
		}
		if !strings.Contains(job.Reason, "project: services/api") {
			t.Fatalf("expected project reason for %s, got %q", job.Name, job.Reason)
		}
		if job.Name != "build:services/api" && !reflect.DeepEqual(job.DependsOn, []string{"build:services/api"}) {
			t.Fatalf("expected dependency on build for %s, got %v", job.Name, job.DependsOn)
		}
		if job.Name == "build:services/api" && len(job.DependsOn) != 0 {
			t.Fatalf("expected no dependencies for %s, got %v", job.Name, job.DependsOn)
		}
	}
}

func TestPlanForGoRootJobNames(t *testing.T) {
	projects := []project{
		{
			Name:       "root",
			Root:       ".",
			Language:   "go",
			ModulePath: "example.com/root",
		},
	}

	impact := analyzeImpact([]string{"main.go"}, projects, false)
	plan := planForGo(impact, "explain", projects)
	if len(plan.Jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(plan.Jobs))
	}
	if plan.Jobs[0].Name != "build" {
		t.Fatalf("unexpected build job name %q", plan.Jobs[0].Name)
	}
	if len(plan.Jobs[0].DependsOn) != 0 {
		t.Fatalf("expected no dependencies for build, got %v", plan.Jobs[0].DependsOn)
	}
}

func TestParseGoWork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "go.work")
	content := `go 1.22

use (
	./services/api
	./libs/lib // comment
)

use ./tools
`
	writeFile(t, path, content)

	modules, err := parseGoWork(path)
	if err != nil {
		t.Fatalf("parse go.work: %v", err)
	}

	expected := []string{"./services/api", "./libs/lib", "./tools"}
	if !reflect.DeepEqual(modules, expected) {
		t.Fatalf("unexpected modules: %v", modules)
	}
}

func TestDiscoverGoProjectsWithGoWork(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.work"), `go 1.22

use (
	./apps/api
	./libs/lib
)
`)

	writeGoMod(t, filepath.Join(dir, "apps", "api"), "example.com/api")
	writeGoMod(t, filepath.Join(dir, "libs", "lib"), "example.com/lib")

	discovery := discoverGoProjects(dir)
	if discovery.dependencyUnknown {
		t.Fatalf("expected dependency graph to be known")
	}
	if len(discovery.projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(discovery.projects))
	}

	names := projectNames(discovery.projects)
	expected := []string{"apps/api", "libs/lib"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("unexpected project names: %v", names)
	}
}

func TestComputeRepoFingerprintChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/root\n\ngo 1.22\n")
	writeFile(t, filepath.Join(dir, "go.sum"), "example.com/root v0.0.0-00010101000000-000000000000 h1:deadbeef\n")

	fingerprintA, err := computeRepoFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}

	writeFile(t, filepath.Join(dir, "go.sum"), "example.com/root v0.0.0-00010101000000-000000000000 h1:cafebabe\n")
	fingerprintB, err := computeRepoFingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint after update: %v", err)
	}

	if fingerprintA == fingerprintB {
		t.Fatalf("expected fingerprint to change")
	}
}

func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()
	content := "module " + modulePath + "\n\ngo 1.22\n"
	writeFile(t, filepath.Join(dir, "go.mod"), content)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
