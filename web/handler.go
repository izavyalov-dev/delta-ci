package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/state"
)

type handler struct {
	service *orchestrator.Service
	tmpl    *templateRegistry
	logger  *slog.Logger
}

func (h *handler) isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func (h *handler) renderError(w http.ResponseWriter, status int, err error) {
	h.logger.Error("handler error", "error", err, "status", status)
	http.Error(w, http.StatusText(status), status)
}

func (h *handler) writeHTML(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(html))
}

// handleDashboard renders the main dashboard (GET /).
func (h *handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	csrfToken := setCSRFCookie(w, r)

	stats, err := h.service.GetSystemStats(ctx)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	recentRuns, _, err := h.service.ListRuns(ctx, orchestrator.RunFilter{Limit: 10})
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	failedRuns, _, err := h.service.ListRuns(ctx, orchestrator.RunFilter{
		State: string(state.RunStateFailed),
		Limit: 5,
	})
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	data := DashboardData{
		Stats: SystemStats{
			Total:     stats.Total,
			Running:   stats.Running,
			Queued:    stats.Queued,
			Succeeded: stats.Succeeded,
			Failed:    stats.Failed,
			Canceled:  stats.Canceled,
		},
		RecentRuns: toRunSummaries(recentRuns),
		FailedRuns: toRunSummaries(failedRuns),
		CSRFToken:  csrfToken,
		CSPNonce:   nonceFromRequest(r),
	}

	if h.isHTMX(r) {
		html, err := h.tmpl.renderFragment("stats", data)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError, err)
			return
		}
		h.writeHTML(w, http.StatusOK, html)
		return
	}

	html, err := h.tmpl.renderPage("dashboard", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

// handleRuns renders the runs list page (GET /runs).
func (h *handler) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	csrfToken := setCSRFCookie(w, r)

	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	limit := 25
	offset := (page - 1) * limit

	filter := orchestrator.RunFilter{
		Repo:   r.URL.Query().Get("repo"),
		State:  r.URL.Query().Get("state"),
		Limit:  limit,
		Offset: offset,
	}

	runs, total, err := h.service.ListRuns(ctx, filter)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	repos, err := h.service.ListDistinctRepos(ctx)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := RunsPageData{
		Runs:  toRunSummaries(runs),
		Repos: repos,
		Filter: RunFilter{
			Repo:  filter.Repo,
			State: filter.State,
		},
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
		CSRFToken:  csrfToken,
		CSPNonce:   nonceFromRequest(r),
	}

	if h.isHTMX(r) {
		html, err := h.tmpl.renderFragment("runs_table", data)
		if err != nil {
			h.renderError(w, http.StatusInternalServerError, err)
			return
		}
		h.writeHTML(w, http.StatusOK, html)
		return
	}

	html, err := h.tmpl.renderPage("runs", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

// handleRunDetail renders a single run (GET /runs/{id}) or handles actions (POST /runs/{id}/cancel, /runs/{id}/rerun).
func (h *handler) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/runs/")
	path = strings.Trim(path, "/")
	parts := strings.SplitN(path, "/", 2)
	runID := parts[0]
	if runID == "" {
		http.NotFound(w, r)
		return
	}

	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if r.Method == http.MethodPost {
		h.handleRunAction(w, r, runID, action)
		return
	}

	if r.Method != http.MethodGet || action != "" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	csrfToken := setCSRFCookie(w, r)

	details, err := h.service.GetRunDetails(ctx, runID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	details = sanitizeRunDetailsForWeb(details)
	active := isActiveRun(details.Run.State)

	data := RunDetailData{
		Details:   details,
		IsActive:  active,
		CSRFToken: csrfToken,
		CSPNonce:  nonceFromRequest(r),
	}

	html, err := h.tmpl.renderPage("run_detail", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

func (h *handler) handleRunAction(w http.ResponseWriter, r *http.Request, runID, action string) {
	if !validateCSRF(r) {
		h.writeHTML(w, http.StatusForbidden, "<p>CSRF validation failed. Please refresh and try again.</p>")
		return
	}

	ctx := r.Context()

	switch action {
	case "cancel":
		_, err := h.service.CancelRun(ctx, runID)
		if err != nil {
			if state.IsTransitionError(err) || errors.Is(err, orchestrator.ErrInvalidRunState) {
				h.writeHTML(w, http.StatusConflict, `<div class="flash flash-error">Cannot cancel this run — it may already be finished.</div>`)
				return
			}
			h.renderError(w, http.StatusInternalServerError, err)
			return
		}
		if h.isHTMX(r) {
			h.writeHTML(w, http.StatusOK, `<div class="flash flash-success">Run cancellation requested.</div>`)
			return
		}
		http.Redirect(w, r, "/runs/"+runID, http.StatusSeeOther)

	case "rerun":
		idempotencyKey := generateNonce()
		details, created, err := h.service.RerunRun(ctx, runID, idempotencyKey)
		if err != nil {
			if state.IsTransitionError(err) {
				h.writeHTML(w, http.StatusConflict, `<div class="flash flash-error">Cannot rerun this run.</div>`)
				return
			}
			h.renderError(w, http.StatusInternalServerError, err)
			return
		}
		newRunID := details.Run.ID
		if !created {
			newRunID = runID
		}
		if h.isHTMX(r) {
			w.Header().Set("HX-Redirect", "/runs/"+newRunID)
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, "/runs/"+newRunID, http.StatusSeeOther)

	default:
		http.NotFound(w, r)
	}
}

// handleNewRun renders and handles the new run form (GET/POST /runs/new).
func (h *handler) handleNewRun(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	csrfToken := setCSRFCookie(w, r)

	if r.Method == http.MethodPost {
		if !validateCSRF(r) {
			h.writeHTML(w, http.StatusForbidden, "<p>CSRF validation failed. Please refresh and try again.</p>")
			return
		}
		if err := r.ParseForm(); err != nil {
			h.renderError(w, http.StatusBadRequest, err)
			return
		}
		repoID := strings.TrimSpace(r.FormValue("repo_id"))
		ref := strings.TrimSpace(r.FormValue("ref"))
		commitSHA := strings.TrimSpace(r.FormValue("commit_sha"))

		if ref != "" && !strings.Contains(ref, "/") {
			ref = "refs/heads/" + ref
		}

		details, err := h.service.CreateRun(ctx, orchestrator.CreateRunRequest{
			RepoID:    repoID,
			Ref:       ref,
			CommitSHA: commitSHA,
		})
		if err != nil {
			repos, _ := h.service.ListDistinctRepos(ctx)
			data := NewRunData{
				Repos:     repos,
				CSRFToken: csrfToken,
				CSPNonce:  nonceFromRequest(r),
				Flash: &FlashMessage{
					Type:    "error",
					Message: err.Error(),
				},
			}
			html, renderErr := h.tmpl.renderPage("new_run", data)
			if renderErr != nil {
				h.renderError(w, http.StatusInternalServerError, renderErr)
				return
			}
			h.writeHTML(w, http.StatusUnprocessableEntity, html)
			return
		}

		http.Redirect(w, r, "/runs/"+details.Run.ID, http.StatusSeeOther)
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	repos, err := h.service.ListDistinctRepos(ctx)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	data := NewRunData{
		Repos:     repos,
		CSRFToken: csrfToken,
		CSPNonce:  nonceFromRequest(r),
	}

	html, err := h.tmpl.renderPage("new_run", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

// handleSettings renders the settings page (GET /settings).
func (h *handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	data := SettingsData{
		Version:  "Phase 5",
		CSPNonce: nonceFromRequest(r),
	}

	html, err := h.tmpl.renderPage("settings", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

// Fragment handlers

func (h *handler) handleFragmentStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stats, err := h.service.GetSystemStats(ctx)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	recentRuns, _, err := h.service.ListRuns(ctx, orchestrator.RunFilter{Limit: 10})
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	failedRuns, _, err := h.service.ListRuns(ctx, orchestrator.RunFilter{
		State: string(state.RunStateFailed),
		Limit: 5,
	})
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	data := DashboardData{
		Stats: SystemStats{
			Total:     stats.Total,
			Running:   stats.Running,
			Queued:    stats.Queued,
			Succeeded: stats.Succeeded,
			Failed:    stats.Failed,
			Canceled:  stats.Canceled,
		},
		RecentRuns: toRunSummaries(recentRuns),
		FailedRuns: toRunSummaries(failedRuns),
	}

	html, err := h.tmpl.renderFragment("stats", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

func (h *handler) handleFragmentRunsTable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	limit := 25
	offset := (page - 1) * limit

	filter := orchestrator.RunFilter{
		Repo:   r.URL.Query().Get("repo"),
		State:  r.URL.Query().Get("state"),
		Limit:  limit,
		Offset: offset,
	}

	runs, total, err := h.service.ListRuns(ctx, filter)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	data := RunsPageData{
		Runs: toRunSummaries(runs),
		Filter: RunFilter{
			Repo:  filter.Repo,
			State: filter.State,
		},
		Total:      total,
		Page:       page,
		TotalPages: totalPages,
	}

	html, err := h.tmpl.renderFragment("runs_table", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

func (h *handler) handleFragmentRunStatus(w http.ResponseWriter, r *http.Request) {
	runID := strings.TrimPrefix(r.URL.Path, "/fragments/run-status/")
	runID = strings.Trim(runID, "/")
	if runID == "" {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	details, err := h.service.GetRunDetails(ctx, runID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	details = sanitizeRunDetailsForWeb(details)

	data := RunDetailData{
		Details:  details,
		IsActive: isActiveRun(details.Run.State),
	}

	html, err := h.tmpl.renderFragment("run_status", data)
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

func (h *handler) handleFragmentJobDetail(w http.ResponseWriter, r *http.Request) {
	jobIndex := strings.TrimPrefix(r.URL.Path, "/fragments/job-detail/")
	jobIndex = strings.Trim(jobIndex, "/")

	// jobIndex is "runID/jobIdx"
	parts := strings.SplitN(jobIndex, "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	runID := parts[0]
	idx, err := strconv.Atoi(parts[1])
	if err != nil || idx < 0 {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()
	details, err := h.service.GetRunDetails(ctx, runID)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}

	details = sanitizeRunDetailsForWeb(details)

	if idx >= len(details.Jobs) {
		http.NotFound(w, r)
		return
	}

	html, err := h.tmpl.renderFragment("job_detail", details.Jobs[idx])
	if err != nil {
		h.renderError(w, http.StatusInternalServerError, err)
		return
	}
	h.writeHTML(w, http.StatusOK, html)
}

// Helpers

func queryInt(r *http.Request, key string, defaultVal int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return v
}

func toRunSummaries(runs []orchestrator.RunSummary) []RunSummary {
	summaries := make([]RunSummary, 0, len(runs))
	for _, r := range runs {
		summaries = append(summaries, RunSummary{
			ID:        r.ID,
			RepoID:    r.RepoID,
			Ref:       r.Ref,
			CommitSHA: r.CommitSHA,
			State:     r.State,
			JobCount:  r.JobCount,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		})
	}
	return summaries
}

// sanitizeRunDetailsForWeb strips lease IDs and normalizes nil slices.
func sanitizeRunDetailsForWeb(details orchestrator.RunDetails) orchestrator.RunDetails {
	for i := range details.Jobs {
		for j := range details.Jobs[i].Attempts {
			details.Jobs[i].Attempts[j].LeaseID = nil
		}
		if details.Jobs[i].Artifacts == nil {
			details.Jobs[i].Artifacts = []state.Artifact{}
		}
		if details.Jobs[i].FailureExplanations == nil {
			details.Jobs[i].FailureExplanations = []state.FailureExplanation{}
		}
		if details.Jobs[i].FailureAIExplanations == nil {
			details.Jobs[i].FailureAIExplanations = []state.FailureAIExplanation{}
		}
		if details.Jobs[i].FixSuggestions == nil {
			details.Jobs[i].FixSuggestions = []state.FixSuggestion{}
		}
	}
	if details.Plan != nil && details.Plan.SkippedJobs == nil {
		details.Plan.SkippedJobs = []state.SkippedJob{}
	}
	return details
}
