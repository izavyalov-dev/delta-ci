package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics collects core counters, histograms, and gauges used by the control plane.
type Metrics struct {
	runs     *prometheus.CounterVec
	jobs     *prometheus.CounterVec
	leases   *prometheus.CounterVec
	failures *prometheus.CounterVec

	runDuration   *prometheus.HistogramVec
	jobDuration   *prometheus.HistogramVec
	queueWait     prometheus.Histogram
	leaseDuration *prometheus.HistogramVec

	queueDepth   prometheus.Gauge
	activeLeases prometheus.Gauge
	deadLetters  prometheus.Counter
	workerActive prometheus.Gauge
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

	runDuration := registerHistogramVec(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "delta_run_duration_seconds",
		Help:    "Duration of runs by final state.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	}, []string{"state"}))

	jobDuration := registerHistogramVec(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "delta_job_duration_seconds",
		Help:    "Duration of jobs by final state.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	}, []string{"state"}))

	queueWait := registerHistogram(registerer, prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "delta_queue_wait_seconds",
		Help:    "Time spent waiting in the dispatch queue.",
		Buckets: prometheus.ExponentialBuckets(0.5, 2, 10),
	}))

	leaseDuration := registerHistogramVec(registerer, prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "delta_lease_duration_seconds",
		Help:    "Duration of leases by final state.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 12),
	}, []string{"state"}))

	queueDepth := registerGauge(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "delta_queue_depth",
		Help: "Current number of items in the dispatch queue.",
	}))

	activeLeaseGauge := registerGauge(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "delta_active_leases",
		Help: "Current number of active leases.",
	}))

	deadLetters := registerCounter(registerer, prometheus.NewCounter(prometheus.CounterOpts{
		Name: "delta_dead_letters_total",
		Help: "Total dead-lettered job attempts.",
	}))

	workerActive := registerGauge(registerer, prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "delta_worker_active",
		Help: "Number of active worker goroutines.",
	}))

	return &Metrics{
		runs:          runs,
		jobs:          jobs,
		leases:        leases,
		failures:      failures,
		runDuration:   runDuration,
		jobDuration:   jobDuration,
		queueWait:     queueWait,
		leaseDuration: leaseDuration,
		queueDepth:    queueDepth,
		activeLeases:  activeLeaseGauge,
		deadLetters:   deadLetters,
		workerActive:  workerActive,
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

func (m *Metrics) ObserveRunDuration(state string, seconds float64) {
	if m == nil || m.runDuration == nil {
		return
	}
	m.runDuration.WithLabelValues(state).Observe(seconds)
}

func (m *Metrics) ObserveJobDuration(state string, seconds float64) {
	if m == nil || m.jobDuration == nil {
		return
	}
	m.jobDuration.WithLabelValues(state).Observe(seconds)
}

func (m *Metrics) ObserveQueueWait(seconds float64) {
	if m == nil || m.queueWait == nil {
		return
	}
	m.queueWait.Observe(seconds)
}

func (m *Metrics) ObserveLeaseDuration(state string, seconds float64) {
	if m == nil || m.leaseDuration == nil {
		return
	}
	m.leaseDuration.WithLabelValues(state).Observe(seconds)
}

func (m *Metrics) SetQueueDepth(depth float64) {
	if m == nil || m.queueDepth == nil {
		return
	}
	m.queueDepth.Set(depth)
}

func (m *Metrics) SetActiveLeases(count float64) {
	if m == nil || m.activeLeases == nil {
		return
	}
	m.activeLeases.Set(count)
}

func (m *Metrics) IncDeadLetters() {
	if m == nil || m.deadLetters == nil {
		return
	}
	m.deadLetters.Inc()
}

func (m *Metrics) SetWorkerActive(count float64) {
	if m == nil || m.workerActive == nil {
		return
	}
	m.workerActive.Set(count)
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

func registerHistogramVec(registerer prometheus.Registerer, histogram *prometheus.HistogramVec) *prometheus.HistogramVec {
	if err := registerer.Register(histogram); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}
	}
	return histogram
}

func registerHistogram(registerer prometheus.Registerer, histogram prometheus.Histogram) prometheus.Histogram {
	if err := registerer.Register(histogram); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(prometheus.Histogram); ok {
				return existing
			}
		}
	}
	return histogram
}

func registerGauge(registerer prometheus.Registerer, gauge prometheus.Gauge) prometheus.Gauge {
	if err := registerer.Register(gauge); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(prometheus.Gauge); ok {
				return existing
			}
		}
	}
	return gauge
}

func registerCounter(registerer prometheus.Registerer, counter prometheus.Counter) prometheus.Counter {
	if err := registerer.Register(counter); err != nil {
		if already, ok := err.(prometheus.AlreadyRegisteredError); ok {
			if existing, ok := already.ExistingCollector.(prometheus.Counter); ok {
				return existing
			}
		}
	}
	return counter
}
