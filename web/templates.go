package web

import (
	"fmt"
	"html/template"
	"io/fs"
	"strings"
	"time"

	"github.com/izavyalov-dev/delta-ci/state"
)

type templateRegistry struct {
	templates map[string]*template.Template
}

func newTemplateRegistry(fsys fs.FS) *templateRegistry {
	funcMap := template.FuncMap{
		"runStateBadge":        runStateBadge,
		"jobStateBadge":        jobStateBadge,
		"validationBadge":      validationBadge,
		"timeAgo":              timeAgo,
		"shortSHA":             shortSHA,
		"add":                  func(a, b int) int { return a + b },
		"sub":                  func(a, b int) int { return a - b },
		"seq":                  seq,
		"diffLines":            diffLines,
		"isActiveRun":          isActiveRun,
		"safeArtifactURI":      safeArtifactURI,
		"failureCategoryLabel": failureCategoryLabel,
	}

	reg := &templateRegistry{
		templates: make(map[string]*template.Template),
	}

	layoutFiles := []string{
		"templates/layout.html",
		"templates/partials/nav.html",
		"templates/partials/pagination.html",
		"templates/partials/flash.html",
		"templates/partials/badges.html",
	}

	type pageSpec struct {
		path      string
		fragments []string // fragment template files also needed
	}

	pages := map[string]pageSpec{
		"dashboard": {
			path:      "templates/pages/dashboard.html",
			fragments: []string{"templates/fragments/stats.html"},
		},
		"runs": {
			path:      "templates/pages/runs.html",
			fragments: []string{"templates/fragments/runs_table.html"},
		},
		"run_detail": {
			path:      "templates/pages/run_detail.html",
			fragments: []string{"templates/fragments/run_status.html", "templates/fragments/job_detail.html"},
		},
		"settings": {
			path: "templates/pages/settings.html",
		},
	}

	fragments := map[string]string{
		"stats":      "templates/fragments/stats.html",
		"runs_table": "templates/fragments/runs_table.html",
		"run_status": "templates/fragments/run_status.html",
		"job_detail": "templates/fragments/job_detail.html",
	}

	for name, spec := range pages {
		files := make([]string, 0, len(layoutFiles)+1+len(spec.fragments))
		files = append(files, layoutFiles...)
		files = append(files, spec.path)
		files = append(files, spec.fragments...)
		t := template.Must(template.New("").Funcs(funcMap).ParseFS(fsys, files...))
		reg.templates[name] = t
	}

	for name, fragPath := range fragments {
		files := []string{
			"templates/partials/badges.html",
			"templates/partials/pagination.html",
			fragPath,
		}
		t := template.Must(template.New("").Funcs(funcMap).ParseFS(fsys, files...))
		reg.templates["fragment:"+name] = t
	}

	return reg
}

func (r *templateRegistry) renderPage(name string, data any) (string, error) {
	t, ok := r.templates[name]
	if !ok {
		return "", fmt.Errorf("template %q not found", name)
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return buf.String(), nil
}

func (r *templateRegistry) renderFragment(name string, data any) (string, error) {
	t, ok := r.templates["fragment:"+name]
	if !ok {
		return "", fmt.Errorf("fragment %q not found", name)
	}
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("render fragment %s: %w", name, err)
	}
	return buf.String(), nil
}

func runStateBadge(s state.RunState) string {
	switch s {
	case state.RunStateSuccess:
		return "success"
	case state.RunStateFailed, state.RunStatePlanFailed:
		return "failed"
	case state.RunStateRunning:
		return "running"
	case state.RunStateQueued:
		return "queued"
	case state.RunStateCanceled, state.RunStateCancelRequested:
		return "canceled"
	case state.RunStatePlanning:
		return "planning"
	case state.RunStateTimeout:
		return "timeout"
	case state.RunStateCreated:
		return "created"
	default:
		return "pending"
	}
}

func jobStateBadge(s state.JobState) string {
	switch s {
	case state.JobStateSucceeded:
		return "success"
	case state.JobStateFailed:
		return "failed"
	case state.JobStateRunning, state.JobStateStarting, state.JobStateUploading:
		return "running"
	case state.JobStateQueued, state.JobStateLeased:
		return "queued"
	case state.JobStateCanceled, state.JobStateCancelRequested:
		return "canceled"
	case state.JobStateTimedOut:
		return "timeout"
	default:
		return "created"
	}
}

func validationBadge(s state.FixSuggestionValidationStatus) string {
	switch s {
	case state.FixSuggestionValidationSucceeded:
		return "validation-succeeded"
	case state.FixSuggestionValidationFailed:
		return "validation-failed"
	case state.FixSuggestionValidationRunning:
		return "validation-running"
	case state.FixSuggestionValidationQueued:
		return "validation-queued"
	case state.FixSuggestionValidationCanceled:
		return "validation-canceled"
	default:
		return "validation-pending"
	}
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func seq(start, end int) []int {
	if end < start {
		return nil
	}
	s := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		s = append(s, i)
	}
	return s
}

// DiffLine represents a single line from a unified diff for template rendering.
type DiffLine struct {
	Class string // "diff-add", "diff-del", "diff-hunk", or ""
	Text  string
}

func diffLines(patch string) []DiffLine {
	lines := strings.Split(patch, "\n")
	result := make([]DiffLine, 0, len(lines))
	for _, line := range lines {
		dl := DiffLine{Text: line}
		switch {
		case strings.HasPrefix(line, "+"):
			dl.Class = "diff-add"
		case strings.HasPrefix(line, "-"):
			dl.Class = "diff-del"
		case strings.HasPrefix(line, "@@"):
			dl.Class = "diff-hunk"
		}
		result = append(result, dl)
	}
	return result
}

func isActiveRun(s state.RunState) bool {
	switch s {
	case state.RunStateCreated, state.RunStatePlanning, state.RunStateQueued, state.RunStateRunning, state.RunStateCancelRequested:
		return true
	default:
		return false
	}
}

func safeArtifactURI(uri string) string {
	if strings.HasPrefix(uri, "https://") || strings.HasPrefix(uri, "s3://") {
		return uri
	}
	return ""
}

func failureCategoryLabel(c state.FailureCategory) string {
	switch c {
	case state.FailureCategoryUser:
		return "User Error"
	case state.FailureCategoryInfra:
		return "Infrastructure"
	case state.FailureCategoryTooling:
		return "Tooling"
	case state.FailureCategoryFlaky:
		return "Flaky"
	case state.FailureCategoryCanceled:
		return "Canceled"
	default:
		return "Unknown"
	}
}
