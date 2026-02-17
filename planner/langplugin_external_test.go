package planner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func writePluginScript(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// mockPluginScript returns a shell script that acts as a language plugin.
// It reads the action from the JSON input and returns appropriate responses.
const mockPluginScript = `#!/bin/sh
# Read stdin into a variable
INPUT=$(cat)
ACTION=$(echo "$INPUT" | sed -n 's/.*"action":"\([^"]*\)".*/\1/p')

case "$ACTION" in
  detect)
    echo '{"version":"1","result":{"detected":true}}'
    ;;
  discover)
    echo '{"version":"1","result":{"projects":[{"name":"root","root":"."}],"code_extensions":[".rs",".toml"],"global_files":["Cargo.toml","Cargo.lock"]}}'
    ;;
  plan)
    echo '{"version":"1","result":{"jobs":[{"name":"build","required":true,"workdir":".","steps":["cargo build"],"reason":"test plan"},{"name":"test","required":true,"workdir":".","steps":["cargo test"],"reason":"test plan","depends_on":["build"]}]}}'
    ;;
  *)
    echo '{"version":"1","error":"unknown action"}' >&2
    exit 1
    ;;
esac
`

func TestExternalPluginDetect(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-rust", mockPluginScript)

	plugin := &ExternalLanguagePlugin{
		PluginName: "rust",
		Path:       script,
	}

	ok, err := plugin.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if !ok {
		t.Fatal("expected detect=true")
	}
}

func TestExternalPluginDetectFalse(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-nope", `#!/bin/sh
cat >/dev/null
echo '{"version":"1","result":{"detected":false}}'
`)

	plugin := &ExternalLanguagePlugin{
		PluginName: "nope",
		Path:       script,
	}

	ok, err := plugin.Detect(context.Background(), dir)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if ok {
		t.Fatal("expected detect=false")
	}
}

func TestExternalPluginDiscover(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-rust", mockPluginScript)

	plugin := &ExternalLanguagePlugin{
		PluginName: "rust",
		Path:       script,
	}

	result, err := plugin.Discover(context.Background(), dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result.Projects))
	}
	if result.Projects[0].Language != "rust" {
		t.Fatalf("expected language=rust, got %s", result.Projects[0].Language)
	}
	if len(result.CodeExtensions) != 2 {
		t.Fatalf("expected 2 code extensions, got %d", len(result.CodeExtensions))
	}
}

func TestExternalPluginPlan(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-rust", mockPluginScript)

	plugin := &ExternalLanguagePlugin{
		PluginName: "rust",
		Path:       script,
	}

	result, err := plugin.Plan(context.Background(), PluginPlanRequest{
		RepoRoot: dir,
		Impact: impactSummary{
			CodeChanges:      true,
			Paths:            []string{"src/main.rs"},
			ImpactedProjects: []string{"root"},
		},
		Projects: []project{
			{Name: "root", Root: ".", Language: "rust"},
		},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(result.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(result.Jobs))
	}
	if result.Jobs[0].Name != "build" {
		t.Fatalf("expected first job=build, got %s", result.Jobs[0].Name)
	}
	if len(result.Jobs[1].DependsOn) == 0 || result.Jobs[1].DependsOn[0] != "build" {
		t.Fatalf("expected test depends on build")
	}
}

func TestExternalPluginErrorResponse(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-bad", `#!/bin/sh
cat >/dev/null
echo '{"version":"1","error":"plugin crashed","result":null}'
`)

	plugin := &ExternalLanguagePlugin{
		PluginName: "bad",
		Path:       script,
	}

	_, err := plugin.Detect(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error from plugin")
	}
}

func TestExternalPluginExitError(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-crash", `#!/bin/sh
exit 1
`)

	plugin := &ExternalLanguagePlugin{
		PluginName: "crash",
		Path:       script,
	}

	_, err := plugin.Detect(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error from plugin that exits non-zero")
	}
}

func TestExternalPluginMultiPluginMerge(t *testing.T) {
	goPlugin := GoLanguagePlugin{}

	dir := t.TempDir()
	writeGoMod(t, dir, "example.com/root")

	goDisc, _ := goPlugin.Discover(context.Background(), dir)

	projects := goDisc.Projects
	// Add a mock "rust" project.
	projects = append(projects, project{Name: "rust-lib", Root: "rust-lib", Language: "rust"})

	impact := analyzeImpact([]string{"main.go", "rust-lib/src/main.rs"}, projects, false)

	// Only Go plugin runs here since we're just testing planWithPlugins merging.
	result := planWithPlugins(context.Background(), []LanguagePlugin{goPlugin}, impact, "test", projects, dir, false)
	if len(result.Jobs) == 0 {
		t.Fatal("expected at least one job from Go plugin")
	}
	// Go plugin should only produce jobs for go-language projects.
	for _, job := range result.Jobs {
		if job.Spec.Workdir == "rust-lib" {
			t.Fatalf("Go plugin should not produce jobs for rust project")
		}
	}
}

func TestPluginFailureFallback(t *testing.T) {
	dir := t.TempDir()
	script := writePluginScript(t, dir, "delta-ci-lang-fail", `#!/bin/sh
exit 1
`)

	failPlugin := &ExternalLanguagePlugin{
		PluginName: "fail",
		Path:       script,
	}

	// The registry should still detect it (conservative on error).
	registry := NewPluginRegistry(failPlugin)
	detected := registry.DetectAll(context.Background(), dir)
	if len(detected) != 1 {
		t.Fatalf("expected detection failure to be treated as detected, got %d", len(detected))
	}
}

func TestDiscoverExternalPlugins(t *testing.T) {
	dir := t.TempDir()
	writePluginScript(t, dir, "delta-ci-lang-python", `#!/bin/sh
echo '{}'
`)
	writePluginScript(t, dir, "delta-ci-lang-rust", `#!/bin/sh
echo '{}'
`)
	// Not a plugin (wrong prefix).
	writePluginScript(t, dir, "other-tool", `#!/bin/sh
echo '{}'
`)

	t.Setenv("PATH", dir)
	plugins := DiscoverExternalPlugins()
	if len(plugins) != 2 {
		t.Fatalf("expected 2 external plugins, got %d", len(plugins))
	}

	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.PluginName] = true
	}
	if !names["python"] || !names["rust"] {
		t.Fatalf("expected python and rust plugins, got %v", names)
	}
}
