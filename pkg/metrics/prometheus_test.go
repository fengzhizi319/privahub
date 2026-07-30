package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestHTTPRequestsTotal_Increment(t *testing.T) {
	HTTPRequestsTotal.WithLabelValues("GET", "/api/test", "200").Inc()

	m := &dto.Metric{}
	if err := HTTPRequestsTotal.WithLabelValues("GET", "/api/test", "200").Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Counter == nil || m.Counter.GetValue() < 1 {
		t.Error("expected counter value >= 1 after Inc()")
	}
}

func TestHTTPRequestDuration_Observe(t *testing.T) {
	HTTPRequestDuration.WithLabelValues("POST", "/api/job").Observe(0.5)

	// Use testutil to collect histogram metrics.
	count := testutil.CollectAndCount(HTTPRequestDuration)
	if count < 1 {
		t.Error("expected at least 1 histogram metric collected")
	}
}

func TestJobExecutionDuration_Observe(t *testing.T) {
	JobExecutionDuration.WithLabelValues("Succeeded").Observe(10.0)
	JobExecutionDuration.WithLabelValues("Failed").Observe(2.0)

	count := testutil.CollectAndCount(JobExecutionDuration)
	if count < 2 {
		t.Errorf("expected at least 2 histogram metrics, got %d", count)
	}
}

func TestActiveGoroutines_Set(t *testing.T) {
	ActiveGoroutines.Set(42)

	m := &dto.Metric{}
	if err := ActiveGoroutines.Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Gauge == nil || m.Gauge.GetValue() != 42 {
		t.Errorf("expected gauge value 42, got %v", m.Gauge.GetValue())
	}

	ActiveGoroutines.Inc()
	if err := ActiveGoroutines.Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Gauge.GetValue() != 43 {
		t.Errorf("expected gauge value 43 after Inc(), got %v", m.Gauge.GetValue())
	}

	ActiveGoroutines.Dec()
	if err := ActiveGoroutines.Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Gauge.GetValue() != 42 {
		t.Errorf("expected gauge value 42 after Dec(), got %v", m.Gauge.GetValue())
	}
}

func TestKusciaReconcileTotal_Increment(t *testing.T) {
	KusciaReconcileTotal.WithLabelValues("DomainRoute", "success").Inc()
	KusciaReconcileTotal.WithLabelValues("DomainRoute", "error").Inc()
	KusciaReconcileTotal.WithLabelValues("KusciaJob", "success").Inc()

	m := &dto.Metric{}
	if err := KusciaReconcileTotal.WithLabelValues("DomainRoute", "success").Write(m); err != nil {
		t.Fatalf("failed to write metric: %v", err)
	}
	if m.Counter == nil || m.Counter.GetValue() < 1 {
		t.Error("expected counter value >= 1 for DomainRoute/success")
	}
}

func TestMetrics_Registered(t *testing.T) {
	// Verify all metrics are registered in the default registry by gathering.
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	expected := map[string]bool{
		"secretpad_http_requests_total":            false,
		"secretpad_http_request_duration_seconds":  false,
		"secretpad_job_execution_duration_seconds": false,
		"secretpad_active_goroutines":              false,
		"secretpad_kuscia_reconcile_total":         false,
	}

	for _, f := range families {
		if _, ok := expected[f.GetName()]; ok {
			expected[f.GetName()] = true
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("metric %q not found in default registry", name)
		}
	}
}
