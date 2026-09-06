package consolidation

import (
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestPackPodsBestFitDecreasing(t *testing.T) {
	nodes := []*v1.NodeInfo{
		{
			Name:        "node-1",
			Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4096}, // 3000m CPU, 12288MB Mem remaining
			PodCount:    2,
		},
		{
			Name:        "node-2",
			Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:   v1.ResourceQuantities{CPU: 2000, Memory: 8192}, // 2000m CPU, 8192MB Mem remaining
			PodCount:    5,
		},
	}

	pods := []*v1.PodInfo{
		{Name: "pod-a", Requested: v1.ResourceQuantities{CPU: 1500, Memory: 4096}},
		{Name: "pod-b", Requested: v1.ResourceQuantities{CPU: 1000, Memory: 2048}},
		{Name: "pod-c", Requested: v1.ResourceQuantities{CPU: 500, Memory: 1024}},
	}

	res := PackPodsBestFitDecreasing(pods, nodes, 110)
	if !res.Success {
		t.Fatalf("expected all pods to fit, but %d pods failed to pack", len(res.UnpackedPods))
	}

	if len(res.Assignments) != 3 {
		t.Errorf("expected 3 assignments, got %d", len(res.Assignments))
	}
}

func TestSelectConsolidationCandidates(t *testing.T) {
	state := &v1.ClusterState{
		Nodes: map[string]*v1.NodeInfo{
			"node-full": {
				Name:        "node-full",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 2000, Memory: 8192}, // 50% load, lots of room
				Ready:       true,
				Schedulable: true,
			},
			"node-underutilized": {
				Name:        "node-underutilized",
				Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
				Requested:   v1.ResourceQuantities{CPU: 400, Memory: 1024}, // 10% load
				Ready:       true,
				Schedulable: true,
			},
		},
		Pods: []*v1.PodInfo{
			{Name: "p1", NodeName: "node-full", Requested: v1.ResourceQuantities{CPU: 2000, Memory: 8192}, IsMovable: true},
			{Name: "p2", NodeName: "node-underutilized", Requested: v1.ResourceQuantities{CPU: 400, Memory: 1024}, IsMovable: true},
		},
	}

	candidates, assignments, err := SelectConsolidationCandidates(state, 0.40)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidates) != 1 || candidates[0] != "node-underutilized" {
		t.Errorf("expected node-underutilized as candidate, got: %v", candidates)
	}

	if assignments["p2"] != "node-full" {
		t.Errorf("expected p2 assigned to node-full, got: %s", assignments["p2"])
	}
}
