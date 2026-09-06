package waterline

import (
	"fmt"
	"math"
	"sort"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// Engine defines the interface for Waterline cluster evaluation and scoring.
type Engine interface {
	Evaluate(state *v1.ClusterState) (*v1.WaterlineReport, error)
	ScoreNode(node *v1.NodeInfo, pods []*v1.PodInfo) (drainScore, safetyScore float64)
	ScorePlacement(pod *v1.PodInfo, candidateNode *v1.NodeInfo) float64
}

type waterlineEngine struct {
	config  v1.WaterlineConfig
	pricing v1.PricingConfig
}

// NewEngine constructs a Waterline evaluation engine with the supplied configuration.
func NewEngine(config v1.WaterlineConfig, pricing v1.PricingConfig) Engine {
	return &waterlineEngine{
		config:  config,
		pricing: pricing,
	}
}

// ScoreNode computes the multi-factor DrainScore and SafetyScore for a single node.
func (e *waterlineEngine) ScoreNode(node *v1.NodeInfo, pods []*v1.PodInfo) (float64, float64) {
	if node.Allocatable.CPU <= 0 || node.Allocatable.Memory <= 0 {
		return 0.0, 0.0
	}

	// 1. Calculate requested load ratios (Kubernetes scheduling invariants)
	cpuLoad := float64(node.Requested.CPU) / float64(node.Allocatable.CPU)
	memLoad := float64(node.Requested.Memory) / float64(node.Allocatable.Memory)

	// Clamp to 1.0 to avoid runaway math in oversubscribed scenarios
	if cpuLoad > 1.0 {
		cpuLoad = 1.0
	}
	if memLoad > 1.0 {
		memLoad = 1.0
	}

	// 2. Compute utilization headroom (the emptier the node, the higher the drain potential)
	cpuHeadroom := 1.0 - cpuLoad
	memHeadroom := 1.0 - memLoad

	// 3. Fragmentation factor: large imbalance between CPU and Memory requests
	fragmentation := math.Abs(cpuLoad - memLoad)

	// 4. Pod mobility & Disruption risk
	totalPods := len(pods)
	movableCount := 0
	pdbProtectedCount := 0

	for _, pod := range pods {
		if pod.IsMovable {
			movableCount++
		}
		if pod.HasPDB {
			pdbProtectedCount++
		}
	}

	mobilityRatio := 1.0
	disruptionRatio := 0.0
	if totalPods > 0 {
		mobilityRatio = float64(movableCount) / float64(totalPods)
		disruptionRatio = float64(totalPods-movableCount+pdbProtectedCount) / float64(totalPods)
	}

	// Consolidation potential combines headroom and workload mobility
	consolidationPotential := ((cpuHeadroom + memHeadroom) / 2.0) * mobilityRatio

	// Multi-factor weighted score calculation
	// High drain score indicates the node is underutilized, fragmented, movable, and low disruption risk
	rawDrain := (e.config.CPUWeight * cpuHeadroom) +
		(e.config.MemoryWeight * memHeadroom) +
		(e.config.ConsolidationWeight * consolidationPotential) +
		(0.05 * fragmentation) -
		(e.config.DisruptionWeight * disruptionRatio)

	// Normalize into 0.0 - 100.0 scale
	drainScore := rawDrain * 100.0
	if drainScore < 0.0 {
		drainScore = 0.0
	}
	if drainScore > 100.0 {
		drainScore = 100.0
	}

	// Round to two decimal places
	drainScore = math.Round(drainScore*100) / 100
	safetyScore := math.Round((100.0-drainScore)*100) / 100

	return drainScore, safetyScore
}

// ScorePlacement evaluates the suitability of a candidate node for receiving a new pod.
// Highly favors nodes expected to remain active (high SafetyScore) and penalizes drain candidates.
func (e *waterlineEngine) ScorePlacement(pod *v1.PodInfo, candidateNode *v1.NodeInfo) float64 {
	// If candidate node cannot fit pod requests, score is 0
	remainingCPU := candidateNode.Allocatable.CPU - candidateNode.Requested.CPU
	remainingMem := candidateNode.Allocatable.Memory - candidateNode.Requested.Memory
	if remainingCPU < pod.Requested.CPU || remainingMem < pod.Requested.Memory {
		return 0.0
	}

	// Calculate target utilization alignment
	targetUtil := e.config.TargetUtilizationThreshold
	projectedCPUUtil := float64(candidateNode.Requested.CPU+pod.Requested.CPU) / float64(candidateNode.Allocatable.CPU)
	projectedMemUtil := float64(candidateNode.Requested.Memory+pod.Requested.Memory) / float64(candidateNode.Allocatable.Memory)

	// Best fit is closest to target without exceeding 0.95
	cpuFit := 1.0 - math.Abs(targetUtil-projectedCPUUtil)
	memFit := 1.0 - math.Abs(targetUtil-projectedMemUtil)
	fitScore := ((cpuFit + memFit) / 2.0) * 100.0

	// Heavy weight on SafetyScore so workloads steer clear of drain candidates
	placementScore := (0.75 * candidateNode.SafetyScore) + (0.25 * fitScore)

	// If node is already marked as a drain candidate, penalize heavily
	if candidateNode.Role == v1.NodeRoleCandidate {
		placementScore *= 0.10 // 90% penalty
	}

	if placementScore < 0.0 {
		placementScore = 0.0
	}
	if placementScore > 100.0 {
		placementScore = 100.0
	}

	return math.Round(placementScore*100) / 100
}

// Evaluate analyzes cluster state, computes node scores, and determines waterline thresholds.
func (e *waterlineEngine) Evaluate(state *v1.ClusterState) (*v1.WaterlineReport, error) {
	if state == nil || len(state.Nodes) == 0 {
		return nil, fmt.Errorf("cannot evaluate empty cluster state")
	}

	// Map pods to their assigned node
	nodePods := make(map[string][]*v1.PodInfo)
	for _, pod := range state.Pods {
		if pod.NodeName != "" {
			nodePods[pod.NodeName] = append(nodePods[pod.NodeName], pod)
		}
	}

	// Calculate scores for each node
	type scoredNode struct {
		node        *v1.NodeInfo
		drainScore  float64
		safetyScore float64
	}

	scoredList := make([]scoredNode, 0, len(state.Nodes))
	for name, node := range state.Nodes {
		if !node.Ready || !node.Schedulable {
			continue
		}
		ds, ss := e.ScoreNode(node, nodePods[name])
		node.DrainScore = ds
		node.SafetyScore = ss
		scoredList = append(scoredList, scoredNode{node: node, drainScore: ds, safetyScore: ss})
	}

	// Sort nodes descending by SafetyScore (safest/highest utilization first)
	sort.Slice(scoredList, func(i, j int) bool {
		return scoredList[i].safetyScore > scoredList[j].safetyScore
	})

	// Vector bin-packing simulation:
	// Starting from the safest nodes, determine how many nodes are strictly needed
	// to pack all movable workloads currently on underutilized nodes.
	safeNodes := make([]string, 0)
	candidateNodes := make([]v1.DrainCandidateInfo, 0)
	neutralNodes := make([]string, 0)

	// Respect minimum nodes floor
	minNodes := e.config.MinNodesFloor
	if minNodes > len(scoredList) {
		minNodes = len(scoredList)
	}

	// Designate top nodes up to floor as safe
	simCapacities := make([]*simNodeCapacity, 0)
	for i := 0; i < minNodes; i++ {
		n := scoredList[i].node
		n.Role = v1.NodeRoleSafe
		safeNodes = append(safeNodes, n.Name)
		simCapacities = append(simCapacities, &simNodeCapacity{
			name:         n.Name,
			remainingCPU: n.Allocatable.CPU - n.Requested.CPU,
			remainingMem: n.Allocatable.Memory - n.Requested.Memory,
		})
	}

	// For the remaining nodes, evaluate if their workloads can fit into designated safe nodes
	for i := minNodes; i < len(scoredList); i++ {
		sn := scoredList[i]
		n := sn.node
		podsOnNode := nodePods[n.Name]

		// Check unmovable pods
		hasUnmovable := false
		movablePods := make([]string, 0)
		unmovablePods := make([]string, 0)
		var movableCPU int64
		var movableMem int64

		for _, p := range podsOnNode {
			if !p.IsMovable {
				hasUnmovable = true
				unmovablePods = append(unmovablePods, p.Name)
			} else {
				movablePods = append(movablePods, p.Name)
				movableCPU += p.Requested.CPU
				movableMem += p.Requested.Memory
			}
		}

		// Node qualifies as candidate if drain score is high and it has no blocking unmovable pods
		canConsolidate := false
		if !hasUnmovable && sn.drainScore >= (e.config.CandidateDrainThreshold*100.0) {
			// Test if all movable pods fit in the simulated safe nodes capacity
			canConsolidate = testFit(podsOnNode, simCapacities)
		}

		if canConsolidate {
			n.Role = v1.NodeRoleCandidate
			// Deduct capacity in safe nodes simulation
			deductFit(podsOnNode, simCapacities)

			reasons := []string{
				fmt.Sprintf("Drain score %.1f exceeds candidate threshold %.1f", sn.drainScore, e.config.CandidateDrainThreshold*100.0),
				fmt.Sprintf("CPU load %.1f%% and Memory load %.1f%% below waterline", (float64(n.Requested.CPU)/float64(n.Allocatable.CPU))*100, (float64(n.Requested.Memory)/float64(n.Allocatable.Memory))*100),
				fmt.Sprintf("All %d pods on node are movable to safe nodes with confirmed replacement capacity", len(movablePods)),
			}

			prob := math.Min(1.0, sn.drainScore/100.0)
			conf := 0.85 // High confidence when replacement capacity verified

			candidateNodes = append(candidateNodes, v1.DrainCandidateInfo{
				NodeName:      n.Name,
				DrainScore:    sn.drainScore,
				SafetyScore:   sn.safetyScore,
				Allocatable:   n.Allocatable,
				Requested:     n.Requested,
				MovablePods:   movablePods,
				UnmovablePods: unmovablePods,
				Probability:   math.Round(prob*100) / 100,
				Confidence:    conf,
				Reasons:       reasons,
			})
		} else {
			// Node cannot be drained; keep as safe or neutral
			if sn.safetyScore > 50.0 {
				n.Role = v1.NodeRoleSafe
				safeNodes = append(safeNodes, n.Name)
				simCapacities = append(simCapacities, &simNodeCapacity{
					name:         n.Name,
					remainingCPU: n.Allocatable.CPU - n.Requested.CPU,
					remainingMem: n.Allocatable.Memory - n.Requested.Memory,
				})
			} else {
				n.Role = v1.NodeRoleNeutral
				neutralNodes = append(neutralNodes, n.Name)
			}
		}
	}

	// Calculate costs
	var currentHourlyCost float64
	var projectedHourlyCost float64
	for _, sn := range scoredList {
		cost := sn.node.HourlyCost
		if cost <= 0 {
			cost = e.pricing.Prices[sn.node.InstanceType]
			if cost <= 0 {
				cost = e.pricing.DefaultPrice
			}
		}
		currentHourlyCost += cost
		if sn.node.Role != v1.NodeRoleCandidate {
			projectedHourlyCost += cost
		}
	}

	// Calculate waste reports
	currentWaste := calculateWaste(state)
	projectedWaste := calculateProjectedWaste(state, candidateNodes)

	report := &v1.WaterlineReport{
		Timestamp:                        time.Now().UTC(),
		WaterlineLevel:                   e.config.TargetUtilizationThreshold,
		SafeNodes:                        safeNodes,
		DrainCandidates:                  candidateNodes,
		NeutralNodes:                     neutralNodes,
		TotalNodes:                       len(scoredList),
		ProjectedNodesAfterConsolidation: len(safeNodes) + len(neutralNodes),
		CurrentHourlyCost:                math.Round(currentHourlyCost*1000) / 1000,
		ProjectedHourlyCost:              math.Round(projectedHourlyCost*1000) / 1000,
		PotentialHourlySavings:           math.Round((currentHourlyCost-projectedHourlyCost)*1000) / 1000,
		CurrentClusterWaste:              currentWaste,
		ProjectedClusterWaste:            projectedWaste,
	}

	return report, nil
}

type simNodeCapacity struct {
	name         string
	remainingCPU int64
	remainingMem int64
}

// testFit checks if all pods can be placed into the available capacity of safe nodes (Best-Fit Decreasing).
func testFit(pods []*v1.PodInfo, capacities []*simNodeCapacity) bool {
	// Create working copies of remaining capacity
	workingCap := make([]int64, len(capacities))
	workingMem := make([]int64, len(capacities))
	for i, c := range capacities {
		workingCap[i] = c.remainingCPU
		workingMem[i] = c.remainingMem
	}

	for _, pod := range pods {
		fitted := false
		for i := range workingCap {
			if workingCap[i] >= pod.Requested.CPU && workingMem[i] >= pod.Requested.Memory {
				workingCap[i] -= pod.Requested.CPU
				workingMem[i] -= pod.Requested.Memory
				fitted = true
				break
			}
		}
		if !fitted {
			return false
		}
	}
	return true
}

func deductFit(pods []*v1.PodInfo, capacities []*simNodeCapacity) {
	for _, pod := range pods {
		for _, c := range capacities {
			if c.remainingCPU >= pod.Requested.CPU && c.remainingMem >= pod.Requested.Memory {
				c.remainingCPU -= pod.Requested.CPU
				c.remainingMem -= pod.Requested.Memory
				break
			}
		}
	}
}

func calculateWaste(state *v1.ClusterState) v1.WasteReport {
	var reqCPU, usedCPU, reqMem, usedMem int64
	for _, node := range state.Nodes {
		reqCPU += node.Requested.CPU
		usedCPU += node.Used.CPU
		reqMem += node.Requested.Memory
		usedMem += node.Used.Memory
	}

	cpuWaste := 0.0
	if reqCPU > 0 {
		diff := reqCPU - usedCPU
		if diff > 0 {
			cpuWaste = (float64(diff) / float64(reqCPU)) * 100.0
		}
	}

	memWaste := 0.0
	if reqMem > 0 {
		diff := reqMem - usedMem
		if diff > 0 {
			memWaste = (float64(diff) / float64(reqMem)) * 100.0
		}
	}

	return v1.WasteReport{
		CPUWastePercent:       math.Round(cpuWaste*100) / 100,
		MemoryWastePercent:    math.Round(memWaste*100) / 100,
		CompositeWastePercent: math.Round(((cpuWaste+memWaste)/2.0)*100) / 100,
		TotalRequestedCPU:     reqCPU,
		TotalUsedCPU:          usedCPU,
		TotalRequestedMemory:  reqMem,
		TotalUsedMemory:       usedMem,
	}
}

func calculateProjectedWaste(state *v1.ClusterState, candidates []v1.DrainCandidateInfo) v1.WasteReport {
	candidateMap := make(map[string]bool)
	for _, c := range candidates {
		candidateMap[c.NodeName] = true
	}

	var reqCPU, usedCPU, reqMem, usedMem int64
	for name, node := range state.Nodes {
		if candidateMap[name] {
			continue // Node eliminated after consolidation
		}
		reqCPU += node.Requested.CPU
		usedCPU += node.Used.CPU
		reqMem += node.Requested.Memory
		usedMem += node.Used.Memory
	}

	cpuWaste := 0.0
	if reqCPU > 0 {
		diff := reqCPU - usedCPU
		if diff > 0 {
			cpuWaste = (float64(diff) / float64(reqCPU)) * 100.0
		}
	}

	memWaste := 0.0
	if reqMem > 0 {
		diff := reqMem - usedMem
		if diff > 0 {
			memWaste = (float64(diff) / float64(reqMem)) * 100.0
		}
	}

	return v1.WasteReport{
		CPUWastePercent:       math.Round(cpuWaste*100) / 100,
		MemoryWastePercent:    math.Round(memWaste*100) / 100,
		CompositeWastePercent: math.Round(((cpuWaste+memWaste)/2.0)*100) / 100,
		TotalRequestedCPU:     reqCPU,
		TotalUsedCPU:          usedCPU,
		TotalRequestedMemory:  reqMem,
		TotalUsedMemory:       usedMem,
	}
}
