package planner

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestGoPluginDetectWithGoMod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/root\n\ngo 1.22\n")

	plugin := GoLanguagePlugin{}
	ok, err := plugin.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !ok {
		t.Fatal("expected detect=true for repo with go.mod")
	}
}

func TestGoPluginDetectWithGoWork(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.work"), "go 1.22\n\nuse ./app\n")

	plugin := GoLanguagePlugin{}
	ok, err := plugin.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !ok {
		t.Fatal("expected detect=true for repo with go.work")
	}
}

func TestGoPluginDetectEmpty(t *testing.T) {
	dir := t.TempDir()

	plugin := GoLanguagePlugin{}
	ok, err := plugin.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if ok {
		t.Fatal("expected detect=false for empty repo")
	}
}

func TestGoPluginDiscover(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "example.com/root")

	plugin := GoLanguagePlugin{}
	result, err := plugin.Discover(context.Background(), dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
	if result.Projects[0].Language != "go" {
		t.Fatalf("expected language=go, got %s", result.Projects[0].Language)
	}

	expectedExts := []string{".go", ".mod", ".sum"}
	sort.Strings(result.CodeExtensions)
	sort.Strings(expectedExts)
	if !reflect.DeepEqual(result.CodeExtensions, expectedExts) {
		t.Fatalf("unexpected code extensions: %v", result.CodeExtensions)
	}
}

func TestGoPluginDiscoverWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.work"), "go 1.22\n\nuse (\n\t./apps/api\n\t./libs/lib\n)\n")
	writeGoMod(t, filepath.Join(dir, "apps", "api"), "example.com/api")
	writeGoMod(t, filepath.Join(dir, "libs", "lib"), "example.com/lib")

	plugin := GoLanguagePlugin{}
	result, err := plugin.Discover(context.Background(), dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(result.Projects))
	}

	names := projectNames(result.Projects)
	expected := []string{"apps/api", "libs/lib"}
	if !reflect.DeepEqual(names, expected) {
		t.Fatalf("unexpected project names: %v", names)
	}
}

func TestGoPluginPlan(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "example.com/root")

	projects := []project{
		{Name: "root", Root: ".", Language: "go", ModulePath: "example.com/root"},
	}
	impact := analyzeImpact([]string{"main.go"}, projects, false)

	plugin := GoLanguagePlugin{}
	result, err := plugin.Plan(context.Background(), PluginPlanRequest{
		RepoRoot:      dir,
		Impact:        impact,
		Explain:       "test explain",
		Projects:      projects,
		CacheReadOnly: false,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(result.Jobs) != 3 {
		t.Fatalf("expected 3 jobs (build/test/lint), got %d", len(result.Jobs))
	}
	if result.Jobs[0].Name != "build" {
		t.Fatalf("expected first job name=build, got %s", result.Jobs[0].Name)
	}
}

func TestGoPluginPlanDocsOnly(t *testing.T) {
	dir := t.TempDir()
	writeGoMod(t, dir, "example.com/root")

	projects := []project{
		{Name: "root", Root: ".", Language: "go", ModulePath: "example.com/root"},
	}
	impact := analyzeImpact([]string{"README.md"}, projects, false)

	plugin := GoLanguagePlugin{}
	result, err := plugin.Plan(context.Background(), PluginPlanRequest{
		RepoRoot:      dir,
		Impact:        impact,
		Explain:       "test",
		Projects:      projects,
		CacheReadOnly: false,
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(result.Jobs) != 1 {
		t.Fatalf("expected 1 job (build only), got %d", len(result.Jobs))
	}
	if len(result.SkippedJobs) != 2 {
		t.Fatalf("expected 2 skipped jobs, got %d", len(result.SkippedJobs))
	}
}
