// Package metrics handles Prometheus instrumentation.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Internal Metrics for Castle Rock Agent observability.
var (
	// Metadata Cache Stats
	MetadataCacheHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "castle_rock_metadata_cache_hits_total",
		Help: "The total number of hits in the Docker metadata cache.",
	})
	MetadataCacheMisses = promauto.NewCounter(prometheus.CounterOpts{
		Name: "castle_rock_metadata_cache_misses_total",
		Help: "The total number of misses in the Docker metadata cache.",
	})

	// Cluster Metrics
	ClusterPushDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "castle_rock_cluster_push_duration_seconds",
		Help:    "Histogram of durations for cluster push operations.",
		Buckets: prometheus.DefBuckets,
	})
	ClusterPushFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "castle_rock_cluster_push_failures_total",
		Help: "The total number of failed cluster push operations.",
	})

	// Runtime Stats
	MonitoredContainers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "castle_rock_monitored_containers_count",
		Help: "The current number of containers being monitored by the agent.",
	})
)
