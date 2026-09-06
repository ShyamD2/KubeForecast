package eviction

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestSafetyValidator_Protections(t *testing.T) {
	val := NewSafetyValidator(15 * time.Minute)

	// 1. Protected kube-system pod
	podSystem := &v1.PodInfo{
		Namespace: "kube-system",
		Name:      "coredns-123",
		IsMovable: true,
	}
	if safe, _ := val.ValidatePod(podSystem); safe {
		t.Errorf("expected system pod to fail safety check")
	}

	// 2. Unmovable DaemonSet pod
	podDaemonSet := &v1.PodInfo{
		Namespace:       "default",
		Name:            "fluentd-abc",
		IsMovable:       false,
		UnmovableReason: "DaemonSet managed",
	}
	if safe, _ := val.ValidatePod(podDaemonSet); safe {
		t.Errorf("expected DaemonSet pod to fail safety check")
	}

	// 3. PDB violation (0 disruptions allowed)
	podPDB := &v1.PodInfo{
		Namespace:          "default",
		Name:               "critical-api-1",
		IsMovable:          true,
		HasPDB:             true,
		DisruptionsAllowed: 0,
	}
	if safe, _ := val.ValidatePod(podPDB); safe {
		t.Errorf("expected PDB with 0 disruptions to fail safety check")
	}

	// 4. Safe movable pod
	podSafe := &v1.PodInfo{
		Namespace:          "treatment",
		Name:               "worker-1",
		IsMovable:          true,
		HasPDB:             true,
		DisruptionsAllowed: 1,
	}
	if safe, reason := val.ValidatePod(podSafe); !safe {
		t.Errorf("expected safe pod to pass, got reason: %s", reason)
	}
}

func TestController_PlanEvictions(t *testing.T) {
	cfg := v1.DefaultEvictionConfig()
	cfg.Enabled = true
	cfg.MaxPodsPerHour = 5
	cfg.MaxConcurrentEvictions = 1
	cfg.MinAvailableCapacityPct = 0.10

	ctrl := NewController(cfg)

	state := &v1.ClusterState{
		Nodes: map[string]*v1.NodeInfo{
			"safe-node": {
				Name:        "safe-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4096},
				Ready:       true,
				Schedulable: true,
			},
			"candidate-node": {
				Name:        "candidate-node",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 500, Memory: 1024},
				Ready:       true,
				Schedulable: true,
			},
		},
		Pods: []*v1.PodInfo{
			{
				Namespace: "default",
				Name:      "movable-pod",
				NodeName:  "candidate-node",
				Requested: v1.ResourceQuantities{CPU: 500, Memory: 1024},
				IsMovable: true,
			},
		},
	}

	candidates := []v1.DrainCandidateInfo{
		{NodeName: "candidate-node"},
	}

	plan, err := ctrl.PlanEvictions(context.Background(), state, candidates, []string{"safe-node"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !plan.Feasible {
		t.Fatalf("expected feasible plan, errors: %v", plan.SafetyErrors)
	}

	if len(plan.Targets) != 1 {
		t.Fatalf("expected 1 eviction target, got %d", len(plan.Targets))
	}

	if plan.Targets[0].Name != "movable-pod" || plan.Targets[0].DestinationNode != "safe-node" {
		t.Errorf("unexpected target: %+v", plan.Targets[0])
	}
}
