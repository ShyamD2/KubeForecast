package prediction

import (
	"testing"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestPredictNode_EmptyNode(t *testing.T) {
	est := NewEstimator(0.40, 5*time.Minute)
	node := &v1.NodeInfo{
		Name:        "empty-node",
		Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
		Requested:   v1.ResourceQuantities{CPU: 0, Memory: 0},
		Ready:       true,
		Schedulable: true,
	}

	pred := est.PredictNode(node, nil, 8000, 32768)
	if !pred.DrainCandidate {
		t.Errorf("expected empty node to be drain candidate")
	}
	if pred.Probability < 0.99 {
		t.Errorf("expected probability ~1.0 for empty node, got %.2f", pred.Probability)
	}
}

func TestPredictNode_UnmovableWorkloadBlocksDrain(t *testing.T) {
	est := NewEstimator(0.40, 5*time.Minute)
	node := &v1.NodeInfo{
		Name:        "node-with-daemonset",
		Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
		Requested:   v1.ResourceQuantities{CPU: 500, Memory: 1024},
		Ready:       true,
		Schedulable: true,
	}
	pods := []*v1.PodInfo{
		{
			Namespace:       "kube-system",
			Name:            "aws-node-xyz",
			IsMovable:       false,
			UnmovableReason: "DaemonSet workload",
		},
	}

	pred := est.PredictNode(node, pods, 8000, 32768)
	if pred.DrainCandidate {
		t.Errorf("node with unmovable DaemonSet pod must never be classified as drain candidate")
	}
	if pred.Probability != 0.0 {
		t.Errorf("expected probability 0.0, got %.2f", pred.Probability)
	}
}

func TestPredictNode_LackingReplacementCapacity(t *testing.T) {
	est := NewEstimator(0.40, 5*time.Minute)
	node := &v1.NodeInfo{
		Name:        "underutilized-node",
		Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
		Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4096}, // 25% load
		Ready:       true,
		Schedulable: true,
	}
	pods := []*v1.PodInfo{
		{Name: "pod-1", IsMovable: true, Requested: v1.ResourceQuantities{CPU: 1000, Memory: 4096}},
	}

	// Cluster only has 500m CPU remaining, cannot absorb 1000m
	pred := est.PredictNode(node, pods, 500, 2048)
	if pred.DrainCandidate {
		t.Errorf("node cannot be drain candidate if cluster lacks replacement capacity")
	}
}
