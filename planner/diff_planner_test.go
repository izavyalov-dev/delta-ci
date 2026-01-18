package planner

import (
	"reflect"
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
