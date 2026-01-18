package planner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

const (
	defaultRepoRootEnv = "DELTA_CI_REPO_ROOT"
)

// DiffPlanner builds a plan using a git diff and simple discovery heuristics.
type DiffPlanner struct {
	RepoRoot string
	Fallback Planner
}

// NewDiffPlanner constructs a diff-aware planner with a fallback.
func NewDiffPlanner(repoRoot string, fallback Planner) DiffPlanner {
	if fallback == nil {
		fallback = StaticPlanner{}
	}
	return DiffPlanner{
		RepoRoot: repoRoot,
		Fallback: fallback,
	}
}

func (p DiffPlanner) Plan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	root, err := resolveRepoRoot(p.RepoRoot)
	if err != nil {
		return p.fallbackPlan(ctx, req, "repo root unavailable", err)
	}

	discovery := discoverInputs(root)
	hasGo := discovery.hasFile("go.mod") || discovery.hasFile("go.sum")

	paths, err := gitChangedFiles(ctx, root, req.CommitSHA)
	if err != nil {
		return p.fallbackPlan(ctx, req, "diff unavailable", err)
	}
	if len(paths) == 0 {
		return p.fallbackPlan(ctx, req, "diff empty", nil)
	}

	if !hasGo {
		return p.fallbackPlan(ctx, req, "no supported build files detected", nil)
	}

	impact := analyzeImpact(paths)
	explain := buildExplain(discovery, impact)

	return planForGo(impact, explain), nil
}

func (p DiffPlanner) fallbackPlan(ctx context.Context, req PlanRequest, reason string, err error) (PlanResult, error) {
	result, planErr := p.Fallback.Plan(ctx, req)
	if planErr != nil {
		return PlanResult{}, planErr
	}
	if err != nil {
		result.Explain = fmt.Sprintf("fallback: %s (%v)", reason, err)
	} else {
		result.Explain = fmt.Sprintf("fallback: %s", reason)
	}
	for i := range result.Jobs {
		if result.Jobs[i].Reason == "" {
			result.Jobs[i].Reason = "fallback plan"
		}
	}
	return result, nil
}

type discoveryInputs struct {
	files map[string]struct{}
}

func discoverInputs(repoRoot string) discoveryInputs {
	candidates := []string{
		"go.mod",
		"go.sum",
		"package.json",
		"Makefile",
		"README.md",
		"CONTRIBUTING.md",
		"ci.ai.yaml",
	}

	found := make(map[string]struct{})
	for _, candidate := range candidates {
		path := filepath.Join(repoRoot, candidate)
		if _, err := os.Stat(path); err == nil {
			found[candidate] = struct{}{}
		}
	}
	return discoveryInputs{files: found}
}

func (d discoveryInputs) hasFile(name string) bool {
	_, ok := d.files[name]
	return ok
}

type impactSummary struct {
	DocsOnly    bool
	Global      bool
	CodeChanges bool
	Paths       []string
}

func analyzeImpact(paths []string) impactSummary {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)

	docsOnly := true
	global := false
	code := false
	for _, path := range paths {
		if !isDocsPath(path) {
			docsOnly = false
		}
		if isGlobalImpact(path) {
			global = true
		}
		if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".mod") || strings.HasSuffix(path, ".sum") {
			code = true
		}
	}
	return impactSummary{
		DocsOnly:    docsOnly,
		Global:      global,
		CodeChanges: code,
		Paths:       paths,
	}
}

func planForGo(impact impactSummary, explain string) PlanResult {
	reasons := buildReasons(impact)
	jobs := []PlannedJob{
		{
			Name:     "build",
			Required: true,
			Spec: protocol.JobSpec{
				Name:    "build",
				Workdir: ".",
				Steps:   []string{"go build ./..."},
			},
			Reason: reasons.build,
		},
	}

	if !impact.DocsOnly {
		jobs = append(jobs, PlannedJob{
			Name:     "test",
			Required: true,
			Spec: protocol.JobSpec{
				Name:    "test",
				Workdir: ".",
				Steps:   []string{"go test ./..."},
			},
			Reason: reasons.test,
		})
		jobs = append(jobs, PlannedJob{
			Name:     "lint",
			Required: false,
			Spec: protocol.JobSpec{
				Name:    "lint",
				Workdir: ".",
				Steps:   []string{"go vet ./..."},
			},
			Reason: reasons.lint,
		})
	}

	return PlanResult{
		Jobs:    jobs,
		Explain: explain,
	}
}

type reasonSet struct {
	build string
	test  string
	lint  string
}

func buildReasons(impact impactSummary) reasonSet {
	impactReason := "non-docs change"
	if impact.DocsOnly {
		impactReason = "docs-only change"
	} else if impact.Global {
		impactReason = "global-impact change"
	} else if impact.CodeChanges {
		impactReason = "code change"
	}

	return reasonSet{
		build: fmt.Sprintf("go build triggered by %s", impactReason),
		test:  fmt.Sprintf("go test triggered by %s", impactReason),
		lint:  fmt.Sprintf("optional lint for %s", impactReason),
	}
}

func buildExplain(discovery discoveryInputs, impact impactSummary) string {
	var b bytes.Buffer
	b.WriteString("diff-aware planner v1: ")
	if len(discovery.files) == 0 {
		b.WriteString("no discovery inputs found")
	} else {
		files := make([]string, 0, len(discovery.files))
		for file := range discovery.files {
			files = append(files, file)
		}
		sort.Strings(files)
		b.WriteString("discovered ")
		b.WriteString(strings.Join(files, ", "))
	}
	b.WriteString("; ")
	b.WriteString("changed paths: ")
	b.WriteString(strings.Join(impact.Paths, ", "))
	if impact.DocsOnly {
		b.WriteString("; docs-only change")
	}
	if impact.Global {
		b.WriteString("; global-impact change")
	} else if !impact.DocsOnly {
		if impact.CodeChanges {
			b.WriteString("; code change")
		} else {
			b.WriteString("; non-docs change")
		}
	}
	return b.String()
}

func gitChangedFiles(ctx context.Context, repoRoot, commitSHA string) ([]string, error) {
	if commitSHA == "" {
		return nil, errors.New("commit sha is required")
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "show", "--name-only", "--pretty=format:", commitSHA)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}
	return paths, nil
}

func resolveRepoRoot(repoRoot string) (string, error) {
	if repoRoot == "" {
		repoRoot = os.Getenv(defaultRepoRootEnv)
	}
	if repoRoot == "" {
		repoRoot = "."
	}
	abs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func isDocsPath(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "docs/") {
		return true
	}
	base := strings.ToLower(filepath.Base(lower))
	if base == "readme.md" || base == "contributing.md" {
		return true
	}
	if strings.HasSuffix(lower, ".md") {
		return true
	}
	return false
}

func isGlobalImpact(path string) bool {
	lower := strings.ToLower(path)
	switch {
	case strings.HasPrefix(lower, ".github/"):
		return true
	case lower == "ci.ai.yaml":
		return true
	case lower == "go.mod", lower == "go.sum":
		return true
	case lower == "makefile":
		return true
	case lower == "dockerfile":
		return true
	default:
		return false
	}
}
