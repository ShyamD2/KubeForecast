package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Webhook metrics
	WebhookRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "predictive_scheduler",
			Subsystem: "webhook",
			Name:      "requests_total",
			Help:      "Total number of admission webhook evaluation requests",
		},
		[]string{"group", "status"},
	)

	WebhookLatencySeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "predictive_scheduler",
			Subsystem: "webhook",
			Name:      "latency_seconds",
			Help:      "Latency of admission webhook evaluations in seconds",
			Buckets:   []float64{0.001, 0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1.0},
		},
		[]string{"group"},
	)

	// Simulation and Prediction metrics
	PredictionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "predictive_scheduler",
			Name:      "predictions_total",
			Help:      "Total number of node predictions computed",
		},
		[]string{"outcome"},
	)

	DrainCandidatesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "drain_candidates",
			Help:      "Current count of nodes predicted as consolidation/drain candidates",
		},
	)

	SafeNodesGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "safe_nodes",
			Help:      "Current count of nodes designated as safe active waterline nodes",
		},
	)

	// Scheduling metrics
	SchedulingDecisionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "predictive_scheduler",
			Name:      "scheduling_decisions_total",
			Help:      "Total number of predictive scheduling placement decisions scored",
		},
		[]string{"node", "role"},
	)

	// Eviction metrics
	EvictionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "predictive_scheduler",
			Name:      "evictions_total",
			Help:      "Total number of predictive evictions executed",
		},
		[]string{"namespace", "reason"},
	)

	EvictionFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "predictive_scheduler",
			Name:      "eviction_failures_total",
			Help:      "Total number of attempted evictions that failed safety checks or PDBs",
		},
		[]string{"reason"},
	)

	// Resource & Waste metrics
	CPURequested = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "cpu_requested",
			Help:      "Total CPU requested by workloads in millicores",
		},
		[]string{"group"},
	)

	CPUUsed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "cpu_used",
			Help:      "Total CPU actually used by workloads in millicores",
		},
		[]string{"group"},
	)

	MemoryRequested = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "memory_requested",
			Help:      "Total memory requested by workloads in bytes",
		},
		[]string{"group"},
	)

	MemoryUsed = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "memory_used",
			Help:      "Total memory actually used by workloads in bytes",
		},
		[]string{"group"},
	)

	ClusterWasteRatio = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "cluster_waste_ratio",
			Help:      "Calculated percentage waste of cluster resources (0-100)",
		},
		[]string{"group", "dimension"}, // dimension: cpu, memory, structural
	)

	EstimatedCost = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "estimated_cost",
			Help:      "Estimated hourly infrastructure cost in USD",
		},
		[]string{"group"},
	)

	EstimatedSavings = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "estimated_savings",
			Help:      "Estimated hourly infrastructure savings in USD achieved by predictive scheduling",
		},
	)

	PredictionAccuracy = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "predictive_scheduler",
			Name:      "prediction_accuracy",
			Help:      "Empirical accuracy ratio of consolidation predictions vs actual drains (0.0 - 1.0)",
		},
	)
)
