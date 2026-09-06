package consolidation

import (
	"fmt"
	"sort"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// PackResult represents the outcome of a vector bin-packing simulation.
type PackResult struct {
	Success     bool
	Assignments map[string]string // pod name -> destination node name
	RemainingCPU map[string]int64
	RemainingMem map[string]int64
	UnpackedPods []*v1.PodInfo
}

// NodeSlot tracks available multidimensional capacity for a node during packing.
type NodeSlot struct {
	Name         string
	Allocatable  v1.ResourceQuantities
	RemainingCPU int64
	RemainingMem int64
	CurrentPods  int
	MaxPods      int
}

// PackPodsBestFitDecreasing attempts to pack a list of pods into the available node slots
// using multidimensional Best-Fit Decreasing (BFD).
func PackPodsBestFitDecreasing(pods []*v1.PodInfo, nodes []*v1.NodeInfo, maxPodsPerNode int) PackResult {
	if maxPodsPerNode <= 0 {
		maxPodsPerNode = 110
	}

	slots := make([]*NodeSlot, 0, len(nodes))
	for _, n := range nodes {
		remCPU := n.Allocatable.CPU - n.Requested.CPU
		remMem := n.Allocatable.Memory - n.Requested.Memory
		if remCPU > 0 && remMem > 0 {
			slots = append(slots, &NodeSlot{
				Name:         n.Name,
				Allocatable:  n.Allocatable,
				RemainingCPU: remCPU,
				RemainingMem: remMem,
				CurrentPods:  n.PodCount,
				MaxPods:      maxPodsPerNode,
			})
		}
	}

	// Sort pods descending by composite resource footprint: (CPU * 4 + Mem)
	sortedPods := make([]*v1.PodInfo, len(pods))
	copy(sortedPods, pods)
	sort.Slice(sortedPods, func(i, j int) bool {
		costI := (sortedPods[i].Requested.CPU * 4) + (sortedPods[i].Requested.Memory / (1024 * 1024))
		costJ := (sortedPods[j].Requested.CPU * 4) + (sortedPods[j].Requested.Memory / (1024 * 1024))
		return costI > costJ
	})

	assignments := make(map[string]string)
	unpacked := make([]*v1.PodInfo, 0)

	for _, pod := range sortedPods {
		bestIdx := -1
		minResidual := int64(1<<62 - 1)

		for i, slot := range slots {
			if slot.CurrentPods >= slot.MaxPods {
				continue
			}
			if slot.RemainingCPU >= pod.Requested.CPU && slot.RemainingMem >= pod.Requested.Memory {
				// Best fit minimizes residual capacity after placement
				residualCPU := slot.RemainingCPU - pod.Requested.CPU
				residualMem := (slot.RemainingMem - pod.Requested.Memory) / (1024 * 1024)
				residual := (residualCPU * 4) + residualMem

				if residual < minResidual {
					minResidual = residual
					bestIdx = i
				}
			}
		}

		if bestIdx >= 0 {
			slot := slots[bestIdx]
			slot.RemainingCPU -= pod.Requested.CPU
			slot.RemainingMem -= pod.Requested.Memory
			slot.CurrentPods++
			assignments[pod.Name] = slot.Name
		} else {
			unpacked = append(unpacked, pod)
		}
	}

	resCPU := make(map[string]int64)
	resMem := make(map[string]int64)
	for _, s := range slots {
		resCPU[s.Name] = s.RemainingCPU
		resMem[s.Name] = s.RemainingMem
	}

	return PackResult{
		Success:      len(unpacked) == 0,
		Assignments:  assignments,
		RemainingCPU: resCPU,
		RemainingMem: resMem,
		UnpackedPods: unpacked,
	}
}

// SelectConsolidationCandidates identifies which underutilized nodes can be completely drained
// such that all their movable workloads are guaranteed to fit onto the remaining active nodes.
func SelectConsolidationCandidates(state *v1.ClusterState, candidateThreshold float64) ([]string, map[string]string, error) {
	if state == nil || len(state.Nodes) == 0 {
		return nil, nil, fmt.Errorf("empty cluster state")
	}

	// Partition nodes into potential candidates and retention pool
	activeNodes := make([]*v1.NodeInfo, 0)
	candidatePool := make([]*v1.NodeInfo, 0)

	nodePods := make(map[string][]*v1.PodInfo)
	for _, p := range state.Pods {
		if p.NodeName != "" {
			nodePods[p.NodeName] = append(nodePods[p.NodeName], p)
		}
	}

	for _, n := range state.Nodes {
		if !n.Ready || !n.Schedulable {
			continue
		}
		cpuLoad := float64(n.Requested.CPU) / float64(n.Allocatable.CPU)
		memLoad := float64(n.Requested.Memory) / float64(n.Allocatable.Memory)
		maxLoad := cpuLoad
		if memLoad > maxLoad {
			maxLoad = memLoad
		}

		// Check if node has unmovable pods
		hasUnmovable := false
		for _, p := range nodePods[n.Name] {
			if !p.IsMovable {
				hasUnmovable = true
				break
			}
		}

		if !hasUnmovable && maxLoad < candidateThreshold {
			candidatePool = append(candidatePool, n)
		} else {
			clone := *n
			activeNodes = append(activeNodes, &clone)
		}
	}

	// Sort candidates ascending by total workload (emptiest node first)
	sort.Slice(candidatePool, func(i, j int) bool {
		loadI := candidatePool[i].Requested.CPU + (candidatePool[i].Requested.Memory / (1024 * 1024))
		loadJ := candidatePool[j].Requested.CPU + (candidatePool[j].Requested.Memory / (1024 * 1024))
		return loadI < loadJ
	})

	drainedNodes := make([]string, 0)
	allAssignments := make(map[string]string)

	// Greedily attempt to drain one candidate at a time into activeNodes
	for _, candidate := range candidatePool {
		pods := nodePods[candidate.Name]
		packResult := PackPodsBestFitDecreasing(pods, activeNodes, 110)
		if packResult.Success {
			drainedNodes = append(drainedNodes, candidate.Name)
			for pName, destNode := range packResult.Assignments {
				allAssignments[pName] = destNode
				// Deduct from active node in our in-memory list
				for _, an := range activeNodes {
					if an.Name == destNode {
						// Find pod
						for _, p := range pods {
							if p.Name == pName {
								an.Requested.CPU += p.Requested.CPU
								an.Requested.Memory += p.Requested.Memory
								an.PodCount++
							}
						}
					}
				}
			}
		} else {
			// Cannot fit; retain candidate as active
			candClone := *candidate
			activeNodes = append(activeNodes, &candClone)
		}
	}

	return drainedNodes, allAssignments, nil
}
