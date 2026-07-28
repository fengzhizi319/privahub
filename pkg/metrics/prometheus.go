// Package metrics provides Prometheus metrics collection for SecretPad-Go.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTPRequestsTotal counts total HTTP requests by method, endpoint, and status.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "secretpad_http_requests_total",
			Help: "Total number of HTTP requests processed",
		},
		[]string{"method", "endpoint", "status"},
	)

	// HTTPRequestDuration tracks HTTP request latency in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "secretpad_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "endpoint"},
	)

	// JobExecutionDuration tracks DAG KusciaJob execution latency.
	JobExecutionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "secretpad_job_execution_duration_seconds",
			Help:    "Execution latency of DAG KusciaJobs in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
		[]string{"status"},
	)

	// ActiveGoroutines tracks current managed goroutine count.
	ActiveGoroutines = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "secretpad_active_goroutines",
			Help: "Current count of active managed goroutines",
		},
	)

	// KusciaReconcileTotal counts Kuscia CRD reconcile operations.
	KusciaReconcileTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "secretpad_kuscia_reconcile_total",
			Help: "Total Kuscia CRD reconcile operations",
		},
		[]string{"resource", "result"},
	)
)
