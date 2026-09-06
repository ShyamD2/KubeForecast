package integration

import (
	"context"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/cluster"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/eviction"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/simulation"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/storage"
)

// TestEndToEndOptimizationLoop verifies the cooperative interactions between
// the Cluster State Collector, Simulation Engine, Score Cache, and Eviction Controller.
func TestEndToEndOptimizationLoop(t *testing.T) {
	pricing := v1.DefaultPricingConfig()
	cfg := v1.DefaultWaterlineConfig()
	cfg.MinNodesFloor = 1
	cfg.CandidateDrainThreshold = 0.40

	store, err := storage.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	scoreCache := scheduler.NewInMemoryScoreCache()
	simEngine := simulation.NewEngine(cfg, pricing, store, scoreCache, nil)
	collector := cluster.NewStateCollector(pricing)

	evictionCfg := v1.DefaultEvictionConfig()
	evictionCfg.Enabled = true
	evictionCfg.MaxPodsPerHour = 10
	evictionCfg.MaxConcurrentEvictions = 2
	evictionCtrl := eviction.NewController(evictionCfg)

	// Construct active multi-node cluster
	nodes := []*v1.NodeInfo{
		{
			Name:         "worker-1",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Ready:        true,
			Schedulable:  true,
		},
		{
			Name:         "worker-2",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Ready:        true,
			Schedulable:  true,
		},
	}

	pods := []*v1.PodInfo{
		{
			Namespace: "treatment",
			Name:      "web-front-1",
			NodeName:  "worker-1",
			Requested: v1.ResourceQuantities{CPU: 2500, Memory: 10000},
			Used:      v1.ResourceQuantities{CPU: 2200, Memory: 8500},
			IsMovable: true,
			HasPDB:    false,
		},
		{
			Namespace: "treatment",
			Name:      "isolated-batch",
			NodeName:  "worker-2",
			Requested: v1.ResourceQuantities{CPU: 400, Memory: 1024},
			Used:      v1.ResourceQuantities{CPU: 200, Memory: 512},
			IsMovable: true,
			HasPDB:    false,
		},
	}

	ctx := context.Background()
	state := collector.BuildClusterState(ctx, nodes, pods)

	// Step 1: Run simulation
	simResult, err := simEngine.RunSimulation(ctx, state)
	if err != nil {
		t.Fatalf("simulation failed: %v", err)
	}

	if len(simResult.Report.SafeNodes) != 1 || simResult.Report.SafeNodes[0] != "worker-1" {
		t.Fatalf("expected worker-1 as safe waterline node, got: %v", simResult.Report.SafeNodes)
	}

	if len(simResult.Report.DrainCandidates) != 1 || simResult.Report.DrainCandidates[0].NodeName != "worker-2" {
		t.Fatalf("expected worker-2 as drain candidate, got: %v", simResult.Report.DrainCandidates)
	}

	// Step 2: Verify scheduler cache has updated
	safeScore, role, found := scoreCache.GetNodeScore("worker-1")
	if !found || role != v1.NodeRoleSafe || safeScore < 65 {
		t.Errorf("expected worker-1 cached as safe node with high score, got role=%s, score=%.1f", role, safeScore)
	}

	candScore, candRole, found := scoreCache.GetNodeScore("worker-2")
	if !found || candRole != v1.NodeRoleCandidate || candScore > 30 {
		t.Errorf("expected worker-2 cached as candidate node with low score, got role=%s, score=%.1f", candRole, candScore)
	}

	// Step 3: Eviction Controller Plans Safe Migration
	plan, err := evictionCtrl.PlanEvictions(ctx, state, simResult.Report.DrainCandidates, simResult.Report.SafeNodes)
	if err != nil {
		t.Fatalf("failed to formulate eviction plan: %v", err)
	}

	if !plan.Feasible {
		t.Fatalf("expected plan to be feasible: %v", plan.SafetyErrors)
	}

	if len(plan.Targets) != 1 {
		t.Fatalf("expected 1 target pod, got %d", len(plan.Targets))
	}

	target := plan.Targets[0]
	if target.Name != "isolated-batch" || target.DestinationNode != "worker-1" {
		t.Errorf("expected isolated-batch moved to worker-1, got destination=%s", target.DestinationNode)
	}
}
