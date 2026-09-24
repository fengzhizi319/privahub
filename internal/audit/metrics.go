package audit

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// metricEvents counts audit events by outcome:
	// persisted | spooled | replayed | dropped.
	metricEvents = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "privahub_audit_events_total",
		Help: "Audit events by outcome (persisted, spooled, replayed, dropped)",
	}, []string{"outcome"})
	metricQueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "privahub_audit_queue_depth",
		Help: "Audit events waiting in the in-memory queue",
	})
	metricSpoolBytes = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "privahub_audit_spool_bytes",
		Help: "Bytes of audit events in the local spool awaiting replay",
	})
	metricAnchors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "privahub_audit_anchor_total",
		Help: "External anchoring attempts by target and result",
	}, []string{"target", "result"})
	metricArchived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "privahub_audit_archived_records_total",
		Help: "Audit records archived to segment files and removed by retention",
	})
)
