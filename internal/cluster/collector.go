package cluster

import (
	"context"
	"strings"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/cost"
)

// StateCollector builds an aggregated ClusterState snapshot from cluster data.
type StateCollector interface {
	BuildClusterState(ctx context.Context, nodes []*v1.NodeInfo, pods []*v1.PodInfo) *v1.ClusterState
	AnalyzePodMobility(pod *v1.PodInfo, isDaemonSet, hasHostPort, hasLocalStorage, isStandalone bool)
}

type collector struct {
	costCalc cost.Calculator
}

// NewStateCollector creates a new cluster state collector.
func NewStateCollector(pricing v1.PricingConfig) StateCollector {
	return &collector{
		costCalc: cost.NewCalculator(pricing),
	}
}

// AnalyzePodMobility inspects pod placement constraints and sets its mobility flags.
func (c *collector) AnalyzePodMobility(pod *v1.PodInfo, isDaemonSet, hasHostPort, hasLocalStorage, isStandalone bool) {
	ns := strings.ToLower(pod.Namespace)

	if ns == "kube-system" || ns == "kube-public" || ns == "kube-node-lease" {
		pod.IsMovable = false
		pod.UnmovableReason = "protected system namespace"
		return
	}

	if isDaemonSet {
		pod.IsMovable = false
		pod.UnmovableReason = "DaemonSet workload cannot be migrated"
		return
	}

	if hasHostPort {
		pod.IsMovable = false
		pod.UnmovableReason = "hostPort constraint locks pod to specific node network"
		return
	}

	if hasLocalStorage {
		pod.IsMovable = false
		pod.UnmovableReason = "local hostPath/ephemeral storage constraint"
		return
	}

	if isStandalone {
		pod.IsMovable = false
		pod.UnmovableReason = "unmanaged standalone pod has no controller replica backing"
		return
	}

	pod.IsMovable = true
	pod.UnmovableReason = ""
}

// BuildClusterState aggregates nodes, pods, and computes cluster-level totals and load ratios.
func (c *collector) BuildClusterState(ctx context.Context, nodes []*v1.NodeInfo, pods []*v1.PodInfo) *v1.ClusterState {
	state := &v1.ClusterState{
		Timestamp: time.Now().UTC(),
		Nodes:     make(map[string]*v1.NodeInfo, len(nodes)),
		Pods:      pods,
	}

	// Index pods by node
	nodePods := make(map[string][]*v1.PodInfo)
	for _, p := range pods {
		if p.NodeName != "" {
			nodePods[p.NodeName] = append(nodePods[p.NodeName], p)
		}
	}

	var totalAllocCPU, totalAllocMem int64
	var totalReqCPU, totalReqMem int64
	var totalUsedCPU, totalUsedMem int64
	var totalCost float64

	for _, n := range nodes {
		// Reset per-node calculated counters
		var nodeReqCPU, nodeReqMem int64
		var nodeUsedCPU, nodeUsedMem int64
		movableCount := 0

		assignedPods := nodePods[n.Name]
		for _, p := range assignedPods {
			nodeReqCPU += p.Requested.CPU
			nodeReqMem += p.Requested.Memory
			nodeUsedCPU += p.Used.CPU
			nodeUsedMem += p.Used.Memory
			if p.IsMovable {
				movableCount++
			}
		}

		n.Requested.CPU = nodeReqCPU
		n.Requested.Memory = nodeReqMem
		n.Used.CPU = nodeUsedCPU
		n.Used.Memory = nodeUsedMem
		n.PodCount = len(assignedPods)
		n.MovablePodCount = movableCount

		if n.Allocatable.CPU > 0 {
			n.CPULoadRatio = float64(nodeReqCPU) / float64(n.Allocatable.CPU)
			n.CPUUtilization = float64(nodeUsedCPU) / float64(n.Allocatable.CPU)
		}
		if n.Allocatable.Memory > 0 {
			n.MemoryLoadRatio = float64(nodeReqMem) / float64(n.Allocatable.Memory)
			n.MemoryUtilization = float64(nodeUsedMem) / float64(n.Allocatable.Memory)
		}

		if n.HourlyCost <= 0 {
			n.HourlyCost = c.costCalc.CalculateNodeCost(n.InstanceType)
		}

		totalAllocCPU += n.Allocatable.CPU
		totalAllocMem += n.Allocatable.Memory
		totalReqCPU += nodeReqCPU
		totalReqMem += nodeReqMem
		totalUsedCPU += nodeUsedCPU
		totalUsedMem += nodeUsedMem
		totalCost += n.HourlyCost

		state.Nodes[n.Name] = n
	}

	state.TotalAllocatable = v1.ResourceQuantities{CPU: totalAllocCPU, Memory: totalAllocMem}
	state.TotalRequested = v1.ResourceQuantities{CPU: totalReqCPU, Memory: totalReqMem}
	state.TotalUsed = v1.ResourceQuantities{CPU: totalUsedCPU, Memory: totalUsedMem}
	state.TotalHourlyCost = totalCost

	return state
}
