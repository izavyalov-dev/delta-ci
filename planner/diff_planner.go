package planner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	hasGo := len(discovery.projects) > 0 || discovery.hasFile("go.mod") || discovery.hasFile("go.sum")

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

	impact := analyzeImpact(paths, discovery.projects, discovery.dependencyUnknown)
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
	files             map[string]struct{}
	projects          []project
	dependencyUnknown bool
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

	projectDiscovery := discoverGoProjects(repoRoot)
	return discoveryInputs{
		files:             found,
		projects:          projectDiscovery.projects,
		dependencyUnknown: projectDiscovery.dependencyUnknown,
	}
}

func (d discoveryInputs) hasFile(name string) bool {
	_, ok := d.files[name]
	return ok
}

type project struct {
	Name       string
	Root       string
	Language   string
	ModulePath string
	Requires   []string
}

type projectDiscovery struct {
	projects          []project
	dependencyUnknown bool
}

type goModInfo struct {
	ModulePath string
	Requires   []string
}

func discoverGoProjects(repoRoot string) projectDiscovery {
	goModPaths, err := findFiles(repoRoot, "go.mod")
	if err != nil {
		return projectDiscovery{dependencyUnknown: true}
	}
	sort.Strings(goModPaths)

	projects := make([]project, 0, len(goModPaths))
	dependencyUnknown := false
	for _, path := range goModPaths {
		info, parseErr := parseGoMod(path)
		if parseErr != nil {
			dependencyUnknown = true
		}

		dir := filepath.Dir(path)
		rel, relErr := filepath.Rel(repoRoot, dir)
		if relErr != nil {
			rel = dir
			dependencyUnknown = true
		}
		rel = normalizeRepoPath(rel)
		name := projectNameFromRoot(rel)

		projects = append(projects, project{
			Name:       name,
			Root:       rel,
			Language:   "go",
			ModulePath: info.ModulePath,
			Requires:   info.Requires,
		})
	}

	return projectDiscovery{
		projects:          projects,
		dependencyUnknown: dependencyUnknown,
	}
}

func parseGoMod(path string) (goModInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return goModInfo{}, err
	}

	info := goModInfo{}
	inRequireBlock := false
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}

		if strings.HasPrefix(line, "module ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				info.ModulePath = fields[1]
			}
			continue
		}

		if strings.HasPrefix(line, "require ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "(" {
				inRequireBlock = true
				continue
			}
			if len(fields) >= 2 {
				info.Requires = append(info.Requires, fields[1])
			}
			continue
		}

		if inRequireBlock {
			if line == ")" {
				inRequireBlock = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				info.Requires = append(info.Requires, fields[0])
			}
		}
	}

	if info.ModulePath == "" {
		return info, fmt.Errorf("module path missing in %s", path)
	}
	return info, nil
}

func findFiles(repoRoot, name string) ([]string, error) {
	var matches []string
	skipDirs := map[string]struct{}{
		".git":        {},
		"node_modules": {},
		"vendor":      {},
		".cache":      {},
	}

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == name {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func normalizeRepoPath(path string) string {
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimSuffix(path, "/")
	if path == "" || path == "." {
		return "."
	}
	return path
}

func projectNameFromRoot(root string) string {
	if root == "." || root == "" {
		return "root"
	}
	return root
}

type impactSummary struct {
	DocsOnly          bool
	Global            bool
	CodeChanges       bool
	Paths             []string
	ImpactedProjects  []string
	UnknownOwnership  []string
	DependencyUnknown bool
}

func analyzeImpact(paths []string, projects []project, dependencyUnknown bool) impactSummary {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)

	docsOnly := true
	global := false
	code := false
	unknownOwnership := false
	var unknownPaths []string
	impacted := make(map[string]struct{})
	projectIndex := buildOwnershipIndex(projects)
	graph := buildDependencyGraph(projects)

	for _, path := range paths {
		if !isDocsPath(path) {
			docsOnly = false
		}
		if isGlobalImpact(path) {
			global = true
		}
		if isCodePath(path) {
			code = true
		}

		if owner := projectIndex.ownerForPath(path); owner != nil {
			impacted[owner.Name] = struct{}{}
		} else {
			unknownOwnership = true
			unknownPaths = append(unknownPaths, path)
		}
	}

	if unknownOwnership || dependencyUnknown {
		global = true
	}

	impactedProjects := resolveImpactedProjects(impacted, projects, global, dependencyUnknown, graph)
	return impactSummary{
		DocsOnly:          docsOnly,
		Global:            global,
		CodeChanges:       code,
		Paths:             paths,
		ImpactedProjects:  impactedProjects,
		UnknownOwnership:  unknownPaths,
		DependencyUnknown: dependencyUnknown,
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
	impactReason := impactReasonSummary(impact)

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
	if len(discovery.projects) > 0 {
		b.WriteString("; projects: ")
		b.WriteString(formatList(projectNames(discovery.projects), 6))
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
	if len(impact.ImpactedProjects) > 0 {
		b.WriteString("; impacted projects: ")
		b.WriteString(formatList(impact.ImpactedProjects, 6))
	}
	if len(impact.UnknownOwnership) > 0 {
		b.WriteString("; unknown ownership: ")
		b.WriteString(formatList(impact.UnknownOwnership, 6))
	}
	if impact.DependencyUnknown {
		b.WriteString("; dependency graph incomplete")
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

func isCodePath(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".mod") || strings.HasSuffix(lower, ".sum")
}

type ownershipIndex struct {
	projects []project
}

func buildOwnershipIndex(projects []project) ownershipIndex {
	if len(projects) == 0 {
		return ownershipIndex{}
	}
	sorted := append([]project(nil), projects...)
	sort.Slice(sorted, func(i, j int) bool {
		ri := sorted[i].Root
		rj := sorted[j].Root
		if ri == "." && rj != "." {
			return false
		}
		if rj == "." && ri != "." {
			return true
		}
		if len(ri) != len(rj) {
			return len(ri) > len(rj)
		}
		return ri < rj
	})
	return ownershipIndex{projects: sorted}
}

func (o ownershipIndex) ownerForPath(path string) *project {
	if len(o.projects) == 0 {
		return nil
	}
	normalized := strings.TrimPrefix(path, "./")
	for i := range o.projects {
		root := o.projects[i].Root
		if root == "." {
			continue
		}
		if normalized == root || strings.HasPrefix(normalized, root+"/") {
			return &o.projects[i]
		}
	}
	for i := range o.projects {
		if o.projects[i].Root == "." {
			return &o.projects[i]
		}
	}
	return nil
}

type dependencyGraph struct {
	dependents map[string][]string
}

func buildDependencyGraph(projects []project) dependencyGraph {
	moduleIndex := make(map[string]string, len(projects))
	for _, project := range projects {
		if project.ModulePath != "" {
			moduleIndex[project.ModulePath] = project.Name
		}
	}

	dependents := make(map[string][]string)
	for _, project := range projects {
		for _, req := range project.Requires {
			if target, ok := moduleIndex[req]; ok {
				dependents[target] = append(dependents[target], project.Name)
			}
		}
	}
	for name := range dependents {
		sort.Strings(dependents[name])
	}
	return dependencyGraph{dependents: dependents}
}

func resolveImpactedProjects(impacted map[string]struct{}, projects []project, global bool, dependencyUnknown bool, graph dependencyGraph) []string {
	if len(projects) == 0 {
		return nil
	}
	if global || dependencyUnknown {
		return projectNames(projects)
	}
	if len(impacted) == 0 {
		return nil
	}
	queue := make([]string, 0, len(impacted))
	for name := range impacted {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, dependent := range graph.dependents[name] {
			if _, ok := impacted[dependent]; ok {
				continue
			}
			impacted[dependent] = struct{}{}
			queue = append(queue, dependent)
		}
	}
	impactedList := make([]string, 0, len(impacted))
	for name := range impacted {
		impactedList = append(impactedList, name)
	}
	sort.Strings(impactedList)
	return impactedList
}

func projectNames(projects []project) []string {
	names := make([]string, 0, len(projects))
	seen := make(map[string]struct{}, len(projects))
	for _, project := range projects {
		if _, ok := seen[project.Name]; ok {
			continue
		}
		seen[project.Name] = struct{}{}
		names = append(names, project.Name)
	}
	sort.Strings(names)
	return names
}

func impactReasonSummary(impact impactSummary) string {
	if impact.DocsOnly {
		return "docs-only change"
	}
	if impact.Global {
		if impact.DependencyUnknown {
			return "global-impact change (dependency graph incomplete)"
		}
		if len(impact.UnknownOwnership) > 0 {
			return "global-impact change (unknown ownership)"
		}
		return "global-impact change"
	}
	if len(impact.ImpactedProjects) > 0 {
		return fmt.Sprintf("change in projects: %s", formatList(impact.ImpactedProjects, 4))
	}
	if impact.CodeChanges {
		return "code change"
	}
	return "non-docs change"
}

func formatList(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, ... (%d total)", strings.Join(items[:limit], ", "), len(items))
}
