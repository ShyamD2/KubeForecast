package simulation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/consolidation"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/cost"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/metrics"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/prediction"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/storage"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/waterline"
)

// SimulationResult encapsulates the deterministic output of a simulation run.
type SimulationResult struct {
	Timestamp          time.Time                 `json:"timestamp"`
	Report             *v1.WaterlineReport       `json:"report"`
	Predictions        map[string]*v1.Prediction `json:"predictions"`
	PlannedAssignments map[string]string         `json:"plannedAssignments"` // pod -> destination node
	SavingsSummary     *cost.SavingsSummary      `json:"savingsSummary"`
	SnapshotURI        string                    `json:"snapshotUri,omitempty"`
	ReportURI          string                    `json:"reportUri,omitempty"`
}

// Engine coordinates deterministic simulation, waterline scoring, predictions, and metrics export.
type Engine interface {
	RunSimulation(ctx context.Context, state *v1.ClusterState) (*SimulationResult, error)
	ScoreCache() scheduler.NodeScoreCache
}

type simulationEngine struct {
	mu           sync.Mutex
	waterlineEng waterline.Engine
	estimator    prediction.Estimator
	costCalc     cost.Calculator
	store        storage.SnapshotStore
	scoreCache   scheduler.NodeScoreCache
	logger       *slog.Logger
	config       v1.WaterlineConfig
	pricing      v1.PricingConfig
}

// NewEngine constructs a deterministic simulation engine.
func NewEngine(
	cfg v1.WaterlineConfig,
	pricing v1.PricingConfig,
	store storage.SnapshotStore,
	scoreCache scheduler.NodeScoreCache,
	logger *slog.Logger,
) Engine {
	if logger == nil {
		logger = slog.Default()
	}
	if scoreCache == nil {
		scoreCache = scheduler.NewInMemoryScoreCache()
	}

	return &simulationEngine{
		waterlineEng: waterline.NewEngine(cfg, pricing),
		estimator:    prediction.NewEstimator(cfg.CandidateDrainThreshold, 5*time.Minute),
		costCalc:     cost.NewCalculator(pricing),
		store:        store,
		scoreCache:   scoreCache,
		logger:       logger,
		config:       cfg,
		pricing:      pricing,
	}
}

func (e *simulationEngine) ScoreCache() scheduler.NodeScoreCache {
	return e.scoreCache
}

// RunSimulation executes a deterministic evaluation pass over the cluster state.
func (e *simulationEngine) RunSimulation(ctx context.Context, state *v1.ClusterState) (*SimulationResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	start := time.Now()
	if state == nil || len(state.Nodes) == 0 {
		return nil, fmt.Errorf("cannot simulate on nil or empty cluster state")
	}

	// 1. Waterline Evaluation
	report, err := e.waterlineEng.Evaluate(state)
	if err != nil {
		e.logger.Error("waterline evaluation failed", "err", err)
		return nil, fmt.Errorf("waterline evaluation error: %w", err)
	}

	// 2. Vector bin-packing consolidation simulation
	candidates, assignments, err := consolidation.SelectConsolidationCandidates(state, e.config.CandidateDrainThreshold)
	if err != nil {
		e.logger.Warn("consolidation candidate selection failed", "err", err)
	}

	// Calculate cluster remaining headroom for prediction estimator
	var safeRemainingCPU, safeRemainingMem int64
	for _, sNodeName := range report.SafeNodes {
		if n, ok := state.Nodes[sNodeName]; ok {
			safeRemainingCPU += (n.Allocatable.CPU - n.Requested.CPU)
			safeRemainingMem += (n.Allocatable.Memory - n.Requested.Memory)
		}
	}

	// Index pods by node
	nodePods := make(map[string][]*v1.PodInfo)
	for _, p := range state.Pods {
		if p.NodeName != "" {
			nodePods[p.NodeName] = append(nodePods[p.NodeName], p)
		}
	}

	// 3. Deterministic Drain Probability Estimation
	predictions := make(map[string]*v1.Prediction, len(state.Nodes))
	for name, node := range state.Nodes {
		pred := e.estimator.PredictNode(node, nodePods[name], safeRemainingCPU, safeRemainingMem)
		predictions[name] = pred

		outcome := "safe"
		if pred.DrainCandidate {
			outcome = "drain_candidate"
		}
		metrics.PredictionsTotal.WithLabelValues(outcome).Inc()
	}

	// 4. Update in-memory score cache used by Kubernetes scheduler plugin
	e.scoreCache.UpdateScores(report, state.Nodes)

	// 5. Separate Control vs Treatment for comparative metrics
	controlNodes := make([]*v1.NodeInfo, 0)
	treatmentNodes := make([]*v1.NodeInfo, 0)
	for _, n := range state.Nodes {
		if n.Role == v1.NodeRoleCandidate {
			// Treatment group excludes drain candidates by consolidating workloads
			controlNodes = append(controlNodes, n)
		} else {
			treatmentNodes = append(treatmentNodes, n)
			controlNodes = append(controlNodes, n)
		}
	}

	savingsSummary := e.costCalc.CalculateExperimentSavings(controlNodes, treatmentNodes, 24*time.Hour)

	// 6. Update Prometheus Gauges
	metrics.DrainCandidatesGauge.Set(float64(len(report.DrainCandidates)))
	metrics.SafeNodesGauge.Set(float64(len(report.SafeNodes)))
	metrics.EstimatedCost.WithLabelValues("cluster").Set(report.CurrentHourlyCost)
	metrics.EstimatedSavings.Set(report.PotentialHourlySavings)

	metrics.ClusterWasteRatio.WithLabelValues("cluster", "cpu").Set(report.CurrentClusterWaste.CPUWastePercent)
	metrics.ClusterWasteRatio.WithLabelValues("cluster", "memory").Set(report.CurrentClusterWaste.MemoryWastePercent)
	metrics.ClusterWasteRatio.WithLabelValues("cluster", "structural").Set(report.CurrentClusterWaste.CompositeWastePercent)

	// 7. Persist to storage (sanitized snapshot + report)
	var snapURI, repURI string
	if e.store != nil {
		snapURI, err = e.store.SaveSnapshot(ctx, state, "all")
		if err != nil {
			e.logger.Warn("failed to persist cluster snapshot", "err", err)
		}
		repURI, err = e.store.SaveReport(ctx, report)
		if err != nil {
			e.logger.Warn("failed to persist waterline report", "err", err)
		}
	}

	e.logger.Info("Simulation completed successfully",
		"safeNodes", len(report.SafeNodes),
		"drainCandidates", len(report.DrainCandidates),
		"potentialHourlySavings", report.PotentialHourlySavings,
		"elapsedMs", time.Since(start).Milliseconds())

	_ = candidates // candidates tracked in report

	return &SimulationResult{
		Timestamp:          time.Now().UTC(),
		Report:             report,
		Predictions:        predictions,
		PlannedAssignments: assignments,
		SavingsSummary:     savingsSummary,
		SnapshotURI:        snapURI,
		ReportURI:          repURI,
	}, nil
}
