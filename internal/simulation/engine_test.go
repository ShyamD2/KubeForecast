package simulation

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/storage"
)

func TestDeterministicSimulation(t *testing.T) {
	cfg := v1.DefaultWaterlineConfig()
	cfg.MinNodesFloor = 1
	cfg.CandidateDrainThreshold = 0.40
	pricing := v1.DefaultPricingConfig()

	store, err := storage.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create temp store: %v", err)
	}

	scoreCache := scheduler.NewInMemoryScoreCache()
	engine := NewEngine(cfg, pricing, store, scoreCache, nil)

	createState := func() *v1.ClusterState {
		return &v1.ClusterState{
			Timestamp: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
			Nodes: map[string]*v1.NodeInfo{
				"node-primary": {
					Name:         "node-primary",
					InstanceType: "m5.large",
					HourlyCost:   0.096,
					Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
					Requested:    v1.ResourceQuantities{CPU: 3000, Memory: 12288}, // 75% load
					Ready:        true,
					Schedulable:  true,
				},
				"node-secondary": {
					Name:         "node-secondary",
					InstanceType: "m5.large",
					HourlyCost:   0.096,
					Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
					Requested:    v1.ResourceQuantities{CPU: 300, Memory: 1024}, // <10% load
					Ready:        true,
					Schedulable:  true,
				},
			},
			Pods: []*v1.PodInfo{
				{
					Namespace: "treatment",
					Name:      "batch-worker",
					NodeName:  "node-secondary",
					Requested: v1.ResourceQuantities{CPU: 300, Memory: 1024},
					IsMovable: true,
				},
			},
		}
	}

	ctx := context.Background()

	// Run 1
	res1, err := engine.RunSimulation(ctx, createState())
	if err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}

	// Run 2 (verify determinism)
	res2, err := engine.RunSimulation(ctx, createState())
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}

	if len(res1.Report.SafeNodes) != len(res2.Report.SafeNodes) {
		t.Fatalf("determinism failure: safe nodes count mismatch")
	}

	if res1.Report.PotentialHourlySavings != res2.Report.PotentialHourlySavings {
		t.Fatalf("determinism failure: potential savings mismatch: %.4f vs %.4f",
			res1.Report.PotentialHourlySavings, res2.Report.PotentialHourlySavings)
	}

	if len(res1.Report.DrainCandidates) != 1 || res1.Report.DrainCandidates[0].NodeName != "node-secondary" {
		t.Errorf("expected node-secondary as candidate, got: %+v", res1.Report.DrainCandidates)
	}

	// Verify ScoreCache was updated for scheduler plugin
	ss, role, found := scoreCache.GetNodeScore("node-secondary")
	if !found {
		t.Errorf("expected score cache entry for node-secondary")
	}
	if role != v1.NodeRoleCandidate {
		t.Errorf("expected candidate role in score cache, got: %s", role)
	}
	if ss > 30.0 {
		t.Errorf("expected low safety score for secondary node, got %.2f", ss)
	}
}
