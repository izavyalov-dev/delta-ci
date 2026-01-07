package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics collects core counters used by the control plane.
type Metrics struct {
	runs     *prometheus.CounterVec
	jobs     *prometheus.CounterVec
	leases   *prometheus.CounterVec
	failures *prometheus.CounterVec
}

func NewMetrics(registerer prometheus.Registerer) *Metrics {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}

	runs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "delta_runs_total",
		Help: "Total runs by state transition.",
	}, []string{"state"})
	jobs := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "delta_jobs_total",
		Help: "Total jobs by state transition.",
	}, []string{"state"})
	leases := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "delta_leases_total",
		Help: "Total leases by state transition.",
	}, []string{"state"})
	failures := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "delta_failures_total",
		Help: "Total failures by type.",
	}, []string{"type"})

	runs = registerCounterVec(registerer, runs)
	jobs = registerCounterVec(registerer, jobs)
	leases = registerCounterVec(registerer, leases)
	failures = registerCounterVec(registerer, failures)

	return &Metrics{
		runs:     runs,
		jobs:     jobs,
		leases:   leases,
		failures: failures,
	}
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

func (m *Metrics) IncRun(state string) {
	if m == nil || m.runs == nil {
		return
	}
	m.runs.WithLabelValues(state).Inc()
}

func (m *Metrics) IncJob(state string) {
	if m == nil || m.jobs == nil {
		return
	}
	m.jobs.WithLabelValues(state).Inc()
}

func (m *Metrics) IncLease(state string) {
	if m == nil || m.leases == nil {
		return
	}
	m.leases.WithLabelValues(state).Inc()
}

func (m *Metrics) IncFailure(kind string) {
	if m == nil || m.failures == nil {
		return
	}
	m.failures.WithLabelValues(kind).Inc()
}

func registerCounterVec(registerer prometheus.Registerer, counter *prometheus.CounterVec) *prometheus.CounterVec {
	if err := registerer.Register(counter); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}
	}
	return counter
}
