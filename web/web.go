// Package web provides the Delta CI web dashboard.
package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/izavyalov-dev/delta-ci/internal/observability"
	"github.com/izavyalov-dev/delta-ci/orchestrator"
)

//go:embed static templates
var embeddedFS embed.FS

// Config controls web dashboard behavior.
type Config struct {
	DevMode bool // serve templates from disk instead of embed
}

// NewHandler creates the web dashboard HTTP handler.
func NewHandler(service *orchestrator.Service, logger *slog.Logger, config Config) http.Handler {
	if logger == nil {
		logger = observability.NewLogger("web")
	}

	var templateFS fs.FS
	if config.DevMode {
		templateFS = nil // templates.go handles disk fallback
	} else {
		templateFS = embeddedFS
	}

	tmpl := newTemplateRegistry(templateFS)

	h := &handler{
		service: service,
		tmpl:    tmpl,
		logger:  logger,
	}

	mux := http.NewServeMux()

	// Static assets
	staticFS, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		panic("web: embedded static fs: " + err.Error())
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Pages
	mux.HandleFunc("/", h.handleDashboard)
	mux.HandleFunc("/runs", h.handleRuns)
	mux.HandleFunc("/runs/", h.handleRunDetail)
	mux.HandleFunc("/settings", h.handleSettings)

	// Fragments (htmx partials)
	mux.HandleFunc("/fragments/stats", h.handleFragmentStats)
	mux.HandleFunc("/fragments/runs-table", h.handleFragmentRunsTable)
	mux.HandleFunc("/fragments/run-status/", h.handleFragmentRunStatus)
	mux.HandleFunc("/fragments/job-detail/", h.handleFragmentJobDetail)

	return cspMiddleware(mux)
}
