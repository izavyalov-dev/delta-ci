package web

import (
	"time"

	"github.com/izavyalov-dev/delta-ci/orchestrator"
	"github.com/izavyalov-dev/delta-ci/state"
)

// RunFilter describes pagination and filtering for run lists.
type RunFilter struct {
	Repo   string
	State  string
	Limit  int
	Offset int
}

// RunSummary is a lightweight run view for list pages.
type RunSummary struct {
	ID        string
	RepoID    string
	Ref       string
	CommitSHA string
	State     state.RunState
	JobCount  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SystemStats aggregates run counts for the dashboard.
type SystemStats struct {
	Total     int
	Running   int
	Queued    int
	Succeeded int
	Failed    int
	Canceled  int
}

// DashboardData is the view model for the dashboard page.
type DashboardData struct {
	Stats      SystemStats
	RecentRuns []RunSummary
	FailedRuns []RunSummary
	CSRFToken  string
	Flash      *FlashMessage
}

// RunsPageData is the view model for the runs list page.
type RunsPageData struct {
	Runs       []RunSummary
	Repos      []string
	Filter     RunFilter
	Total      int
	Page       int
	TotalPages int
	CSRFToken  string
	Flash      *FlashMessage
}

// RunDetailData is the view model for the run detail page.
type RunDetailData struct {
	Details   orchestrator.RunDetails
	IsActive  bool
	CSRFToken string
	Flash     *FlashMessage
}

// SettingsData is the view model for the settings page.
type SettingsData struct {
	Version string
	Flash   *FlashMessage
}

// FlashMessage represents a transient user notification.
type FlashMessage struct {
	Type    string // "success" or "error"
	Message string
}
