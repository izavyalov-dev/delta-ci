package planner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	Recipes  RecipeStore
}

// NewDiffPlanner constructs a diff-aware planner with a fallback.
func NewDiffPlanner(repoRoot string, fallback Planner, recipes RecipeStore) DiffPlanner {
	if fallback == nil {
		fallback = StaticPlanner{}
	}
	return DiffPlanner{
		RepoRoot: repoRoot,
		Fallback: fallback,
		Recipes:  recipes,
	}
}

func (p DiffPlanner) Plan(ctx context.Context, req PlanRequest) (PlanResult, error) {
	root, err := resolveRepoRoot(p.RepoRoot)
	if err != nil {
		result, planErr := p.fallbackPlan(ctx, req, "repo root unavailable", err)
		if planErr != nil {
			return PlanResult{}, planErr
		}
		result.RecipeSource = PlanSourceFallback
		return result, nil
	}

	discovery := discoverInputs(root)
	fingerprint, fingerprintErr := computeRepoFingerprint(root)
	hasGo := len(discovery.projects) > 0 || discovery.hasFile("go.mod") || discovery.hasFile("go.sum") || discovery.hasFile("go.work") || discovery.hasFile("go.work.sum")
	cacheReadOnly := isPullRequestRef(req.Ref)

	if discovery.hasFile("ci.ai.yaml") {
		result, planErr := p.fallbackPlan(ctx, req, "explicit config present (ci.ai.yaml)", nil)
		if planErr != nil {
			return PlanResult{}, planErr
		}
		applyPlanMetadata(&result, fingerprint, fingerprintErr, PlanSourceConfig, "")
		return result, nil
	}

	if !hasGo {
		result, planErr := p.fallbackPlan(ctx, req, "no supported build files detected", nil)
		if planErr != nil {
			return PlanResult{}, planErr
		}
		applyPlanMetadata(&result, fingerprint, fingerprintErr, PlanSourceFallback, "")
		return result, nil
	}

	recipeResult, recipeUsed, recipeNote, err := p.planFromRecipe(ctx, req, discovery, root, fingerprint, fingerprintErr)
	if err != nil {
		return PlanResult{}, err
	}
	if recipeUsed {
		return recipeResult, nil
	}

	paths, err := gitChangedFiles(ctx, root, req.CommitSHA)
	if err != nil {
		result, planErr := p.fallbackPlan(ctx, req, "diff unavailable", err)
		if planErr != nil {
			return PlanResult{}, planErr
		}
		applyPlanMetadata(&result, fingerprint, fingerprintErr, PlanSourceFallback, recipeNote)
		return result, nil
	}
	if len(paths) == 0 {
		result, planErr := p.fallbackPlan(ctx, req, "diff empty", nil)
		if planErr != nil {
			return PlanResult{}, planErr
		}
		applyPlanMetadata(&result, fingerprint, fingerprintErr, PlanSourceFallback, recipeNote)
		return result, nil
	}

	impact := analyzeImpact(paths, discovery.projects, discovery.dependencyUnknown)
	explain := buildExplain(discovery, impact)

	result := planForGo(impact, explain, discovery.projects, root, cacheReadOnly)
	applyPlanMetadata(&result, fingerprint, fingerprintErr, PlanSourceDiscovery, recipeNote)
	return result, nil
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

func (p DiffPlanner) planFromRecipe(ctx context.Context, req PlanRequest, discovery discoveryInputs, repoRoot, fingerprint string, fingerprintErr error) (PlanResult, bool, string, error) {
	if p.Recipes == nil {
		return PlanResult{}, false, "", nil
	}
	if fingerprint == "" {
		if fingerprintErr != nil {
			return PlanResult{}, false, "recipe lookup skipped: fingerprint unavailable", nil
		}
		return PlanResult{}, false, "recipe lookup skipped: fingerprint missing", nil
	}

	recipe, ok, err := p.Recipes.FindRecipe(ctx, req.RepoID, fingerprint)
	if err != nil {
		return PlanResult{}, false, fmt.Sprintf("recipe lookup failed: %v", err), nil
	}
	if !ok {
		return PlanResult{}, false, "no recipe matched fingerprint", nil
	}

	paths, diffErr := gitChangedFiles(ctx, repoRoot, req.CommitSHA)
	explain := buildRecipeExplain(discovery, paths, diffErr, recipe, fingerprint)
	result := PlanResult{
		Jobs:          jobsFromRecipe(recipe),
		Explain:       explain,
		Fingerprint:   fingerprint,
		RecipeSource:  PlanSourceRecipe,
		RecipeID:      recipe.ID,
		RecipeVersion: recipe.Version,
	}
	applyPlanMetadata(&result, "", fingerprintErr, "", "")
	return result, true, "", nil
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
		"go.work",
		"go.work.sum",
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
	goWorkPath := filepath.Join(repoRoot, "go.work")
	if info, err := os.Stat(goWorkPath); err == nil && !info.IsDir() {
		modules, parseErr := parseGoWork(goWorkPath)
		if parseErr != nil || len(modules) == 0 {
			fallback := discoverGoProjectsFromGoMod(repoRoot)
			fallback.dependencyUnknown = true
			return fallback
		}
		workspace := discoverGoProjectsFromGoWork(repoRoot, modules)
		if len(workspace.projects) > 0 {
			return workspace
		}
		fallback := discoverGoProjectsFromGoMod(repoRoot)
		fallback.dependencyUnknown = true
		return fallback
	} else if err != nil && !os.IsNotExist(err) {
		fallback := discoverGoProjectsFromGoMod(repoRoot)
		fallback.dependencyUnknown = true
		return fallback
	}

	return discoverGoProjectsFromGoMod(repoRoot)
}

func discoverGoProjectsFromGoMod(repoRoot string) projectDiscovery {
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

func discoverGoProjectsFromGoWork(repoRoot string, modules []string) projectDiscovery {
	projects := make([]project, 0, len(modules))
	dependencyUnknown := false
	for _, modulePath := range modules {
		if modulePath == "" {
			dependencyUnknown = true
			continue
		}

		abs, rel, ok := resolveGoWorkModule(repoRoot, modulePath)
		if !ok {
			dependencyUnknown = true
			continue
		}

		info := goModInfo{}
		goModPath := filepath.Join(abs, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			var parseErr error
			info, parseErr = parseGoMod(goModPath)
			if parseErr != nil {
				dependencyUnknown = true
			}
		} else {
			dependencyUnknown = true
		}

		projects = append(projects, project{
			Name:       projectNameFromRoot(rel),
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

func resolveGoWorkModule(repoRoot, modulePath string) (string, string, bool) {
	abs := modulePath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, modulePath)
	}
	abs = filepath.Clean(abs)

	rel, err := filepath.Rel(repoRoot, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", "", false
	}
	rel = normalizeRepoPath(rel)
	return abs, rel, true
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

func parseGoWork(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var modules []string
	inUseBlock := false
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

		if strings.HasPrefix(line, "use ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "(" {
				inUseBlock = true
				continue
			}
			if len(fields) >= 2 {
				modules = append(modules, fields[1])
			}
			continue
		}

		if inUseBlock {
			if line == ")" {
				inUseBlock = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				modules = append(modules, fields[0])
			}
		}
	}

	return uniqueStrings(modules), nil
}

func computeRepoFingerprint(repoRoot string) (string, error) {
	names := []string{
		"go.mod",
		"go.sum",
		"go.work",
		"go.work.sum",
		"package.json",
		"package-lock.json",
		"pnpm-lock.yaml",
		"yarn.lock",
		"Makefile",
		"justfile",
		"Taskfile.yml",
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"settings.gradle",
		"settings.gradle.kts",
		"Cargo.toml",
		"WORKSPACE",
		"BUILD.bazel",
		"Dockerfile",
		"ci.ai.yaml",
	}

	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	paths, err := findFilesByNames(repoRoot, nameSet)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", errors.New("no fingerprint inputs found")
	}

	sort.Strings(paths)
	hasher := sha256.New()
	for _, path := range paths {
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return "", err
		}
		rel = normalizeRepoPath(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte(rel)); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return "", err
		}
		if _, err := hasher.Write(data); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	unique := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		unique = append(unique, item)
	}
	return unique
}

func findFiles(repoRoot, name string) ([]string, error) {
	var matches []string
	skipDirs := map[string]struct{}{
		".git":         {},
		"node_modules": {},
		"vendor":       {},
		".cache":       {},
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

func findFilesByNames(repoRoot string, names map[string]struct{}) ([]string, error) {
	var matches []string
	skipDirs := map[string]struct{}{
		".git":         {},
		"node_modules": {},
		"vendor":       {},
		".cache":       {},
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
		if _, ok := names[d.Name()]; ok {
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

func formatPaths(paths []string, limit int) string {
	if len(paths) == 0 {
		return "(none)"
	}
	return formatList(paths, limit)
}

func shortFingerprint(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8]
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

func planForGo(impact impactSummary, explain string, projects []project, repoRoot string, cacheReadOnly bool) PlanResult {
	reasons := buildReasons(impact)
	projectRoots := buildProjectRootIndex(projects)
	targetProjects := impact.ImpactedProjects
	if len(targetProjects) == 0 {
		targetProjects = projectNames(projects)
	}
	if len(targetProjects) == 0 {
		targetProjects = []string{"root"}
		if _, ok := projectRoots["root"]; !ok {
			projectRoots["root"] = "."
		}
	}
	sort.Strings(targetProjects)

	jobs := make([]PlannedJob, 0, len(targetProjects))
	skipped := buildSkippedJobs(impact, targetProjects, projectRoots)
	for _, projectName := range targetProjects {
		root := projectRoots[projectName]
		if root == "" {
			root = "."
		}
		cacheSpecs := buildGoCacheSpecs(repoRoot, root, cacheReadOnly)
		buildName := jobNameForProject("build", projectName, root)
		jobs = append(jobs, PlannedJob{
			Name:     buildName,
			Required: true,
			Spec: protocol.JobSpec{
				Name:    buildName,
				Workdir: root,
				Steps:   []string{"go build ./..."},
				Caches:  cacheSpecs,
			},
			Reason: reasonForProject(reasons.build, projectName, root),
		})

		if !impact.DocsOnly {
			testName := jobNameForProject("test", projectName, root)
			jobs = append(jobs, PlannedJob{
				Name:     testName,
				Required: true,
				Spec: protocol.JobSpec{
					Name:    testName,
					Workdir: root,
					Steps:   []string{"go test ./..."},
					Caches:  cacheSpecs,
				},
				Reason:    reasonForProject(reasons.test, projectName, root),
				DependsOn: []string{buildName},
			})
			lintName := jobNameForProject("lint", projectName, root)
			jobs = append(jobs, PlannedJob{
				Name:     lintName,
				Required: false,
				Spec: protocol.JobSpec{
					Name:    lintName,
					Workdir: root,
					Steps:   []string{"go vet ./..."},
					Caches:  cacheSpecs,
				},
				Reason:    reasonForProject(reasons.lint, projectName, root),
				DependsOn: []string{buildName},
			})
		}
	}

	return PlanResult{
		Jobs:        jobs,
		Explain:     explain,
		SkippedJobs: skipped,
	}
}

type reasonSet struct {
	build string
	test  string
	lint  string
}

type skipReasonSet struct {
	test string
	lint string
}

func buildReasons(impact impactSummary) reasonSet {
	impactReason := impactReasonSummary(impact)

	return reasonSet{
		build: fmt.Sprintf("go build triggered by %s", impactReason),
		test:  fmt.Sprintf("go test triggered by %s", impactReason),
		lint:  fmt.Sprintf("optional lint for %s", impactReason),
	}
}

func buildSkipReasons() skipReasonSet {
	return skipReasonSet{
		test: "docs-only change: skipped go test",
		lint: "docs-only change: skipped go vet",
	}
}

func buildSkippedJobs(impact impactSummary, projects []string, roots map[string]string) []SkippedJob {
	if !impact.DocsOnly {
		return nil
	}
	reasons := buildSkipReasons()
	skipped := make([]SkippedJob, 0, len(projects)*2)
	for _, projectName := range projects {
		root := roots[projectName]
		if root == "" {
			root = "."
		}
		testName := jobNameForProject("test", projectName, root)
		skipped = append(skipped, SkippedJob{
			Name:   testName,
			Reason: reasonForProject(reasons.test, projectName, root),
		})
		lintName := jobNameForProject("lint", projectName, root)
		skipped = append(skipped, SkippedJob{
			Name:   lintName,
			Reason: reasonForProject(reasons.lint, projectName, root),
		})
	}
	return skipped
}

func buildExplain(discovery discoveryInputs, impact impactSummary) string {
	var b bytes.Buffer
	b.WriteString("diff-aware planner v1: ")
	appendDiscoveryExplain(&b, discovery)
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

func appendDiscoveryExplain(b *bytes.Buffer, discovery discoveryInputs) {
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
}

func buildRecipeExplain(discovery discoveryInputs, paths []string, diffErr error, recipe Recipe, fingerprint string) string {
	var b bytes.Buffer
	b.WriteString("diff-aware planner v1: ")
	appendDiscoveryExplain(&b, discovery)
	if diffErr != nil {
		b.WriteString("; diff unavailable")
	} else {
		b.WriteString("; changed paths: ")
		b.WriteString(formatPaths(paths, 6))
	}
	if fingerprint != "" {
		b.WriteString("; fingerprint ")
		b.WriteString(shortFingerprint(fingerprint))
	}
	b.WriteString("; recipe ")
	b.WriteString(recipe.ID)
	if recipe.Version > 0 {
		fmt.Fprintf(&b, " v%d", recipe.Version)
	}
	return b.String()
}

func jobsFromRecipe(recipe Recipe) []PlannedJob {
	jobs := make([]PlannedJob, 0, len(recipe.Jobs))
	reason := fmt.Sprintf("recipe %s v%d", recipe.ID, recipe.Version)
	for _, job := range recipe.Jobs {
		job.Reason = reason
		jobs = append(jobs, job)
	}
	return jobs
}

func applyPlanMetadata(result *PlanResult, fingerprint string, fingerprintErr error, source string, note string) {
	if result == nil {
		return
	}
	if fingerprint != "" {
		result.Fingerprint = fingerprint
	}
	if source != "" {
		result.RecipeSource = source
	}
	if fingerprintErr != nil {
		result.Explain = appendExplain(result.Explain, fmt.Sprintf("fingerprint unavailable: %v", fingerprintErr))
	}
	if note != "" {
		result.Explain = appendExplain(result.Explain, note)
	}
}

func appendExplain(base, note string) string {
	if note == "" {
		return base
	}
	if base == "" {
		return note
	}
	return base + "; " + note
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
	case lower == "go.work", lower == "go.work.sum":
		return true
	case lower == "makefile":
		return true
	case lower == "dockerfile":
		return true
	default:
		return false
	}
}

func isPullRequestRef(ref string) bool {
	return strings.HasPrefix(ref, "refs/pull/")
}

func buildGoCacheSpecs(repoRoot, projectRoot string, readOnly bool) []protocol.CacheSpec {
	key, err := goModuleCacheKey(repoRoot, projectRoot)
	if err != nil {
		return nil
	}
	return []protocol.CacheSpec{
		{
			Type:     "deps",
			Key:      key,
			Paths:    []string{"~/go/pkg/mod"},
			ReadOnly: readOnly,
		},
	}
}

func goModuleCacheKey(repoRoot, projectRoot string) (string, error) {
	moduleRoot := projectRoot
	if moduleRoot == "" {
		moduleRoot = "."
	}

	files := []string{"go.mod", "go.sum"}
	hasher := sha256.New()
	found := false
	for _, name := range files {
		path := filepath.Join(repoRoot, moduleRoot, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		found = true
		if _, err := hasher.Write([]byte(name)); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return "", err
		}
		if _, err := hasher.Write(data); err != nil {
			return "", err
		}
		if _, err := hasher.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	if !found {
		return "", errors.New("go module files unavailable")
	}
	return fmt.Sprintf("go:deps:%s", hex.EncodeToString(hasher.Sum(nil))), nil
}

func buildProjectRootIndex(projects []project) map[string]string {
	roots := make(map[string]string, len(projects))
	for _, project := range projects {
		roots[project.Name] = project.Root
	}
	return roots
}

func jobNameForProject(base, projectName, root string) string {
	if isRootProject(projectName, root) {
		return base
	}
	return fmt.Sprintf("%s:%s", base, projectName)
}

func reasonForProject(reason, projectName, root string) string {
	if isRootProject(projectName, root) {
		return reason
	}
	return fmt.Sprintf("%s (project: %s)", reason, projectName)
}

func isRootProject(projectName, root string) bool {
	return projectName == "root" || root == "."
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
