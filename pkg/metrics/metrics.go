package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	SubmissionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cee_submissions_total",
			Help: "Total number of code submissions processed",
		},
		[]string{"status", "language"},
	)

	SubmissionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cee_submission_duration_seconds",
			Help:    "Execution duration of submissions in seconds",
			Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"language"},
	)

	QueueSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cee_queue_size",
			Help: "Current number of submissions waiting in queue",
		},
	)

	QueueProcessing = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cee_queue_processing",
			Help: "Current number of submissions actively being processed",
		},
	)

	WorkersActive = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cee_workers_active",
			Help: "Number of active workers available to process jobs",
		},
	)
)

// RecordSubmission increments the submissions counter and records execution duration.
func RecordSubmission(status string, language string, durationSeconds float64) {
	SubmissionsTotal.WithLabelValues(status, language).Inc()
	if durationSeconds > 0 {
		SubmissionDuration.WithLabelValues(language).Observe(durationSeconds)
	}
}

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}
