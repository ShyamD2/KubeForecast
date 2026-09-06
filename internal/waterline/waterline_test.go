package waterline

import (
	"testing"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestScoreNode(t *testing.T) {
	cfg := v1.DefaultWaterlineConfig()
	pricing := v1.DefaultPricingConfig()
	engine := NewEngine(cfg, pricing)

	// Node 1: High utilization (safe)
	nodeSafe := &v1.NodeInfo{
		Name:         "node-safe",
		InstanceType: "m5.large",
		Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8192},
		Requested:    v1.ResourceQuantities{CPU: 1600, Memory: 6553}, // ~80% load
		Ready:        true,
		Schedulable:  true,
	}
	podsSafe := []*v1.PodInfo{
		{Name: "pod-1", IsMovable: true, HasPDB: false},
		{Name: "pod-2", IsMovable: true, HasPDB: true},
	}

	dsSafe, ssSafe := engine.ScoreNode(nodeSafe, podsSafe)
	if dsSafe > 35.0 {
		t.Errorf("expected low drain score for heavily loaded node, got %.2f", dsSafe)
	}
	if ssSafe < 65.0 {
		t.Errorf("expected high safety score for heavily loaded node, got %.2f", ssSafe)
	}

	// Node 2: Low utilization (candidate)
	nodeEmpty := &v1.NodeInfo{
		Name:         "node-empty",
		InstanceType: "m5.large",
		Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8192},
		Requested:    v1.ResourceQuantities{CPU: 200, Memory: 800}, // ~10% load
		Ready:        true,
		Schedulable:  true,
	}
	podsEmpty := []*v1.PodInfo{
		{Name: "pod-3", IsMovable: true, HasPDB: false},
	}

	dsEmpty, ssEmpty := engine.ScoreNode(nodeEmpty, podsEmpty)
	if dsEmpty < 50.0 {
		t.Errorf("expected high drain score for empty node, got %.2f", dsEmpty)
	}
	if ssEmpty > 50.0 {
		t.Errorf("expected lower safety score for empty node, got %.2f", ssEmpty)
	}
}

func TestScorePlacement(t *testing.T) {
	cfg := v1.DefaultWaterlineConfig()
	pricing := v1.DefaultPricingConfig()
	engine := NewEngine(cfg, pricing)

	pod := &v1.PodInfo{
		Name:      "new-pod",
		Requested: v1.ResourceQuantities{CPU: 200, Memory: 500},
	}

	safeNode := &v1.NodeInfo{
		Name:        "node-safe",
		SafetyScore: 85.0,
		Role:        v1.NodeRoleSafe,
		Allocatable: v1.ResourceQuantities{CPU: 2000, Memory: 8192},
		Requested:   v1.ResourceQuantities{CPU: 1200, Memory: 5000},
	}

	candidateNode := &v1.NodeInfo{
		Name:        "node-candidate",
		SafetyScore: 20.0,
		Role:        v1.NodeRoleCandidate,
		Allocatable: v1.ResourceQuantities{CPU: 2000, Memory: 8192},
		Requested:   v1.ResourceQuantities{CPU: 200, Memory: 1000},
	}

	safeScore := engine.ScorePlacement(pod, safeNode)
	candidateScore := engine.ScorePlacement(pod, candidateNode)

	if safeScore <= candidateScore {
		t.Errorf("placement on safe node (%.2f) must score higher than candidate node (%.2f)", safeScore, candidateScore)
	}
}

func TestEvaluateClusterState(t *testing.T) {
	cfg := v1.DefaultWaterlineConfig()
	cfg.MinNodesFloor = 1
	cfg.CandidateDrainThreshold = 0.40
	pricing := v1.DefaultPricingConfig()
	engine := NewEngine(cfg, pricing)

	state := &v1.ClusterState{
		Timestamp: time.Now(),
		Nodes: map[string]*v1.NodeInfo{
			"node-1": {
				Name:         "node-1",
				InstanceType: "m5.large",
				HourlyCost:   0.096,
				Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
				Requested:    v1.ResourceQuantities{CPU: 1500, Memory: 6000}, // 75% load
				Ready:        true,
				Schedulable:  true,
			},
			"node-2": {
				Name:         "node-2",
				InstanceType: "m5.large",
				HourlyCost:   0.096,
				Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
				Requested:    v1.ResourceQuantities{CPU: 200, Memory: 500}, // 10% load
				Ready:        true,
				Schedulable:  true,
			},
		},
		Pods: []*v1.PodInfo{
			{Name: "p1", NodeName: "node-1", Requested: v1.ResourceQuantities{CPU: 1500, Memory: 6000}, IsMovable: true},
			{Name: "p2", NodeName: "node-2", Requested: v1.ResourceQuantities{CPU: 200, Memory: 500}, IsMovable: true},
		},
	}

	report, err := engine.Evaluate(state)
	if err != nil {
		t.Fatalf("unexpected evaluate error: %v", err)
	}

	if len(report.SafeNodes) != 1 || report.SafeNodes[0] != "node-1" {
		t.Errorf("expected node-1 as safe node, got: %v", report.SafeNodes)
	}

	if len(report.DrainCandidates) != 1 || report.DrainCandidates[0].NodeName != "node-2" {
		t.Errorf("expected node-2 as drain candidate, got: %v", report.DrainCandidates)
	}

	if report.PotentialHourlySavings <= 0 {
		t.Errorf("expected potential hourly savings > 0, got %.4f", report.PotentialHourlySavings)
	}
}
