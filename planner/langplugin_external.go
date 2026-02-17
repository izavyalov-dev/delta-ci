package planner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/izavyalov-dev/delta-ci/protocol"
)

const (
	externalPluginPrefix  = "delta-ci-lang-"
	externalPluginVersion = "1"
	defaultPluginTimeout  = 30 * time.Second
)

// ExternalLanguagePlugin wraps an external executable that implements the
// language plugin protocol via JSON over stdin/stdout.
type ExternalLanguagePlugin struct {
	// PluginName is the short language name (e.g. "rust").
	PluginName string

	// Path is the absolute path to the plugin executable.
	Path string

	// Timeout is the maximum time a plugin invocation may take.
	// Defaults to 30s if zero.
	Timeout time.Duration
}

// externalRequest is the JSON envelope sent to a plugin on stdin.
type externalRequest struct {
	Action  string          `json:"action"`
	Version string          `json:"version"`
	Request json.RawMessage `json:"request"`
}

// externalResponse is the JSON envelope received from a plugin on stdout.
type externalResponse struct {
	Version string          `json:"version"`
	Error   string          `json:"error,omitempty"`
	Result  json.RawMessage `json:"result"`
}

// externalDetectRequest is sent for the "detect" action.
type externalDetectRequest struct {
	RepoRoot string `json:"repo_root"`
}

// externalDetectResult is the result of the "detect" action.
type externalDetectResult struct {
	Detected bool `json:"detected"`
}

// externalDiscoverRequest is sent for the "discover" action.
type externalDiscoverRequest struct {
	RepoRoot string `json:"repo_root"`
}

// externalDiscoverResult is the result of the "discover" action.
type externalDiscoverResult struct {
	Projects          []externalProject `json:"projects"`
	DependencyUnknown bool              `json:"dependency_unknown"`
	CodeExtensions    []string          `json:"code_extensions"`
	GlobalFiles       []string          `json:"global_files"`
	FingerprintFiles  []string          `json:"fingerprint_files"`
}

type externalProject struct {
	Name       string   `json:"name"`
	Root       string   `json:"root"`
	ModulePath string   `json:"module_path,omitempty"`
	Requires   []string `json:"requires,omitempty"`
}

// externalPlanRequest is sent for the "plan" action.
type externalPlanRequest struct {
	RepoRoot      string            `json:"repo_root"`
	Projects      []externalProject `json:"projects"`
	CacheReadOnly bool              `json:"cache_read_only"`
	Impact        externalImpact    `json:"impact"`
}

type externalImpact struct {
	DocsOnly          bool     `json:"docs_only"`
	Global            bool     `json:"global"`
	CodeChanges       bool     `json:"code_changes"`
	Paths             []string `json:"paths"`
	ImpactedProjects  []string `json:"impacted_projects"`
	DependencyUnknown bool     `json:"dependency_unknown"`
}

// externalPlanResult is the result of the "plan" action.
type externalPlanResult struct {
	Jobs        []externalJob        `json:"jobs"`
	SkippedJobs []externalSkippedJob `json:"skipped_jobs,omitempty"`
}

type externalJob struct {
	Name      string   `json:"name"`
	Required  bool     `json:"required"`
	Workdir   string   `json:"workdir"`
	Steps     []string `json:"steps"`
	Reason    string   `json:"reason"`
	DependsOn []string `json:"depends_on,omitempty"`
}

type externalSkippedJob struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

func (p *ExternalLanguagePlugin) Name() string { return p.PluginName }

func (p *ExternalLanguagePlugin) Detect(ctx context.Context, repoRoot string) (bool, error) {
	reqData, _ := json.Marshal(externalDetectRequest{RepoRoot: repoRoot})
	respData, err := p.invoke(ctx, "detect", reqData)
	if err != nil {
		return false, err
	}
	var result externalDetectResult
	if err := json.Unmarshal(respData, &result); err != nil {
		return false, fmt.Errorf("external plugin %s: unmarshal detect result: %w", p.PluginName, err)
	}
	return result.Detected, nil
}

func (p *ExternalLanguagePlugin) Discover(ctx context.Context, repoRoot string) (DiscoverResult, error) {
	reqData, _ := json.Marshal(externalDiscoverRequest{RepoRoot: repoRoot})
	respData, err := p.invoke(ctx, "discover", reqData)
	if err != nil {
		return DiscoverResult{}, err
	}
	var result externalDiscoverResult
	if err := json.Unmarshal(respData, &result); err != nil {
		return DiscoverResult{}, fmt.Errorf("external plugin %s: unmarshal discover result: %w", p.PluginName, err)
	}

	projects := make([]project, len(result.Projects))
	for i, ep := range result.Projects {
		projects[i] = project{
			Name:       ep.Name,
			Root:       ep.Root,
			Language:   p.PluginName,
			ModulePath: ep.ModulePath,
			Requires:   ep.Requires,
		}
	}
	return DiscoverResult{
		Projects:          projects,
		DependencyUnknown: result.DependencyUnknown,
		CodeExtensions:    result.CodeExtensions,
		GlobalFiles:       result.GlobalFiles,
		FingerprintFiles:  result.FingerprintFiles,
	}, nil
}

func (p *ExternalLanguagePlugin) Plan(ctx context.Context, req PluginPlanRequest) (PluginPlanResult, error) {
	extProjects := make([]externalProject, len(req.Projects))
	for i, proj := range req.Projects {
		extProjects[i] = externalProject{
			Name:       proj.Name,
			Root:       proj.Root,
			ModulePath: proj.ModulePath,
			Requires:   proj.Requires,
		}
	}
	extReq := externalPlanRequest{
		RepoRoot:      req.RepoRoot,
		Projects:      extProjects,
		CacheReadOnly: req.CacheReadOnly,
		Impact: externalImpact{
			DocsOnly:          req.Impact.DocsOnly,
			Global:            req.Impact.Global,
			CodeChanges:       req.Impact.CodeChanges,
			Paths:             req.Impact.Paths,
			ImpactedProjects:  req.Impact.ImpactedProjects,
			DependencyUnknown: req.Impact.DependencyUnknown,
		},
	}

	reqData, _ := json.Marshal(extReq)
	respData, err := p.invoke(ctx, "plan", reqData)
	if err != nil {
		return PluginPlanResult{}, err
	}

	var result externalPlanResult
	if err := json.Unmarshal(respData, &result); err != nil {
		return PluginPlanResult{}, fmt.Errorf("external plugin %s: unmarshal plan result: %w", p.PluginName, err)
	}

	jobs := make([]PlannedJob, len(result.Jobs))
	for i, ej := range result.Jobs {
		jobs[i] = PlannedJob{
			Name:     ej.Name,
			Required: ej.Required,
			Spec: protocol.JobSpec{
				Name:    ej.Name,
				Workdir: ej.Workdir,
				Steps:   ej.Steps,
			},
			Reason:    ej.Reason,
			DependsOn: ej.DependsOn,
		}
	}

	skipped := make([]SkippedJob, len(result.SkippedJobs))
	for i, es := range result.SkippedJobs {
		skipped[i] = SkippedJob{Name: es.Name, Reason: es.Reason}
	}

	return PluginPlanResult{Jobs: jobs, SkippedJobs: skipped}, nil
}

func (p *ExternalLanguagePlugin) invoke(ctx context.Context, action string, requestData json.RawMessage) (json.RawMessage, error) {
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = defaultPluginTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	envelope := externalRequest{
		Action:  action,
		Version: externalPluginVersion,
		Request: requestData,
	}
	input, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("external plugin %s: marshal request: %w", p.PluginName, err)
	}

	cmd := exec.CommandContext(ctx, p.Path)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("external plugin %s (%s %s): %w: %s", p.PluginName, p.Path, action, err, stderr.String())
	}

	var resp externalResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("external plugin %s: unmarshal response: %w", p.PluginName, err)
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("external plugin %s: %s", p.PluginName, resp.Error)
	}
	return resp.Result, nil
}

// DiscoverExternalPlugins scans PATH for executables matching
// delta-ci-lang-<name> and returns ExternalLanguagePlugin instances for each.
func DiscoverExternalPlugins() []*ExternalLanguagePlugin {
	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	seen := make(map[string]struct{})
	var plugins []*ExternalLanguagePlugin

	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, externalPluginPrefix) {
				continue
			}
			langName := strings.TrimPrefix(name, externalPluginPrefix)
			if langName == "" {
				continue
			}
			if _, ok := seen[langName]; ok {
				continue
			}
			seen[langName] = struct{}{}
			plugins = append(plugins, &ExternalLanguagePlugin{
				PluginName: langName,
				Path:       filepath.Join(dir, name),
			})
		}
	}
	return plugins
}
