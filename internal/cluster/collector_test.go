package cluster

import (
	"context"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestAnalyzePodMobility(t *testing.T) {
	c := NewStateCollector(v1.DefaultPricingConfig())

	// 1. Kube-system pod
	pSys := &v1.PodInfo{Namespace: "kube-system", Name: "kube-proxy"}
	c.AnalyzePodMobility(pSys, false, false, false, false)
	if pSys.IsMovable {
		t.Errorf("system pod must not be movable")
	}

	// 2. DaemonSet pod
	pDS := &v1.PodInfo{Namespace: "default", Name: "node-exporter"}
	c.AnalyzePodMobility(pDS, true, false, false, false)
	if pDS.IsMovable {
		t.Errorf("DaemonSet pod must not be movable")
	}

	// 3. HostPort pod
	pHP := &v1.PodInfo{Namespace: "default", Name: "ingress-controller"}
	c.AnalyzePodMobility(pHP, false, true, false, false)
	if pHP.IsMovable {
		t.Errorf("HostPort pod must not be movable")
	}

	// 4. Regular deployment pod
	pOK := &v1.PodInfo{Namespace: "default", Name: "frontend-abc"}
	c.AnalyzePodMobility(pOK, false, false, false, false)
	if !pOK.IsMovable {
		t.Errorf("deployment pod should be movable")
	}
}

func TestBuildClusterState(t *testing.T) {
	c := NewStateCollector(v1.DefaultPricingConfig())

	nodes := []*v1.NodeInfo{
		{
			Name:         "node-1",
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 2000, Memory: 8000},
			Ready:        true,
			Schedulable:  true,
		},
	}
	pods := []*v1.PodInfo{
		{
			Namespace: "default",
			Name:      "pod-1",
			NodeName:  "node-1",
			Requested: v1.ResourceQuantities{CPU: 500, Memory: 2000},
			Used:      v1.ResourceQuantities{CPU: 250, Memory: 1000},
			IsMovable: true,
		},
	}

	state := c.BuildClusterState(context.Background(), nodes, pods)
	if state.TotalAllocatable.CPU != 2000 {
		t.Errorf("expected 2000 allocatable CPU, got %d", state.TotalAllocatable.CPU)
	}
	if state.TotalRequested.CPU != 500 {
		t.Errorf("expected 500 requested CPU, got %d", state.TotalRequested.CPU)
	}
	if state.Nodes["node-1"].CPULoadRatio != 0.25 {
		t.Errorf("expected 0.25 CPU load ratio, got %.2f", state.Nodes["node-1"].CPULoadRatio)
	}
}
