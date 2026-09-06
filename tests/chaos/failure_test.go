package chaos

import (
	"context"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/admission"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/eviction"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/simulation"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/storage"
)

// TestFailure_SimulatorUnavailable ensures that if the simulator has not run or is down,
// the scheduler plugin gracefully falls back to neutral scores without blocking pod placement.
func TestFailure_SimulatorUnavailable(t *testing.T) {
	// Completely empty cache simulates a dead or unreachable simulator
	emptyCache := scheduler.NewInMemoryScoreCache()
	plugin := scheduler.NewPlugin(emptyCache, 1)

	pod := &v1.PodInfo{Name: "critical-app"}
	score, err := plugin.ScoreNode(context.Background(), pod, "unknown-node")
	if err != nil {
		t.Fatalf("scheduler plugin must not error when cache is empty: %v", err)
	}

	if score != 50 {
		t.Errorf("expected neutral fallback score (50), got %d", score)
	}
}

// TestFailure_WebhookUnavailableFailSafe verifies that malformed or failing webhook requests
// never block admission and always fail-open (Allowed = true).
func TestFailure_WebhookUnavailableFailSafe(t *testing.T) {
	handler := admission.NewHandler(nil)

	// Simulate broken admission payload
	req := &admission.AdmissionReviewRequest{
		Kind:       "AdmissionReview",
		APIVersion: "admission.k8s.io/v1",
	}
	req.Request.UID = "test-uid-fail"
	req.Request.Object = []byte(`{"invalid-json"`)

	resp := handler.MutatePod(req)
	if !resp.Response.Allowed {
		t.Errorf("webhook MUST fail open (allowed=true) on unmarshal failure to preserve cluster availability")
	}
}

// TestFailure_S3UnavailableGracefulFallback verifies that when S3 persistence is down or unconfigured,
// simulation finishes cleanly and falls back to local storage without panicking or halting.
func TestFailure_S3UnavailableGracefulFallback(t *testing.T) {
	localStore, err := storage.NewLocalFileStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// S3 store pointing to invalid/non-existent bucket
	s3Store := storage.NewS3SnapshotStore("", "us-east-1", "test", localStore)

	cfg := v1.DefaultWaterlineConfig()
	pricing := v1.DefaultPricingConfig()
	engine := simulation.NewEngine(cfg, pricing, s3Store, nil, nil)

	state := &v1.ClusterState{
		Nodes: map[string]*v1.NodeInfo{
			"node-1": {
				Name:        "node-1",
				Allocatable: v1.ResourceQuantities{CPU: 2000, Memory: 8000},
				Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4000},
				Ready:       true,
				Schedulable: true,
			},
		},
	}

	res, err := engine.RunSimulation(context.Background(), state)
	if err != nil {
		t.Fatalf("simulation must succeed even when S3 is unavailable: %v", err)
	}

	if res.Report == nil {
		t.Errorf("expected valid waterline report despite S3 fallback")
	}
}

// TestFailure_PDBBlocksEviction verifies that PodDisruptionBudget violations
// immediately abort planned evictions.
func TestFailure_PDBBlocksEviction(t *testing.T) {
	cfg := v1.DefaultEvictionConfig()
	cfg.Enabled = true
	ctrl := eviction.NewController(cfg)

	state := &v1.ClusterState{
		Nodes: map[string]*v1.NodeInfo{
			"safe-node": {
				Name:        "safe-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4096},
				Ready:       true,
				Schedulable: true,
			},
			"cand-node": {
				Name:        "cand-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 500, Memory: 1024},
				Ready:       true,
				Schedulable: true,
			},
		},
		Pods: []*v1.PodInfo{
			{
				Namespace:          "production",
				Name:               "payment-gateway-pod",
				NodeName:           "cand-node",
				Requested:          v1.ResourceQuantities{CPU: 500, Memory: 1024},
				IsMovable:          true,
				HasPDB:             true,
				DisruptionsAllowed: 0, // PDB STRICTLY PROHIBITS DISRUPTION
			},
		},
	}

	candidates := []v1.DrainCandidateInfo{{NodeName: "cand-node"}}
	plan, err := ctrl.PlanEvictions(context.Background(), state, candidates, []string{"safe-node"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Targets) > 0 {
		t.Fatalf("PDB with 0 disruptions allowed MUST block eviction, but %d targets planned", len(plan.Targets))
	}
}

// TestFailure_SuddenWorkloadSpikeYieldsToAvailability ensures that if safe nodes lack headroom
// due to sudden traffic/scaling, eviction of candidates is suppressed.
func TestFailure_SuddenWorkloadSpikeYieldsToAvailability(t *testing.T) {
	cfg := v1.DefaultEvictionConfig()
	cfg.Enabled = true
	cfg.MinAvailableCapacityPct = 0.20 // Require at least 20% free headroom
	ctrl := eviction.NewController(cfg)

	// Safe node has suffered a sudden workload spike: 95% utilized
	state := &v1.ClusterState{
		Nodes: map[string]*v1.NodeInfo{
			"spiked-node": {
				Name:        "spiked-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 3800, Memory: 15500}, // 95% load
				Ready:       true,
				Schedulable: true,
			},
			"cand-node": {
				Name:        "cand-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 400, Memory: 1024},
				Ready:       true,
				Schedulable: true,
			},
		},
		Pods: []*v1.PodInfo{
			{
				Namespace: "default",
				Name:      "batch-pod",
				NodeName:  "cand-node",
				Requested: v1.ResourceQuantities{CPU: 400, Memory: 1024},
				IsMovable: true,
			},
		},
	}

	candidates := []v1.DrainCandidateInfo{{NodeName: "cand-node"}}
	plan, err := ctrl.PlanEvictions(context.Background(), state, candidates, []string{"spiked-node"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(plan.Targets) > 0 {
		t.Fatalf("optimization must yield to availability: eviction should be blocked when safe nodes lack headroom")
	}
}
