package prediction

import (
	"fmt"
	"math"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// Estimator predicts node consolidation probability using deterministic cluster signals.
type Estimator interface {
	PredictNode(node *v1.NodeInfo, pods []*v1.PodInfo, clusterRemainingCPU, clusterRemainingMem int64) *v1.Prediction
}

type deterministicEstimator struct {
	candidateThreshold float64
	minGracePeriod     time.Duration
}

// NewEstimator constructs a transparent deterministic prediction engine.
func NewEstimator(candidateThreshold float64, minGracePeriod time.Duration) Estimator {
	if candidateThreshold <= 0 {
		candidateThreshold = 0.40
	}
	if minGracePeriod <= 0 {
		minGracePeriod = 5 * time.Minute
	}
	return &deterministicEstimator{
		candidateThreshold: candidateThreshold,
		minGracePeriod:     minGracePeriod,
	}
}

// PredictNode estimates P(node will be consolidated within prediction window) with transparent reasons.
func (e *deterministicEstimator) PredictNode(node *v1.NodeInfo, pods []*v1.PodInfo, clusterRemainingCPU, clusterRemainingMem int64) *v1.Prediction {
	timestamp := time.Now().UTC()
	reasons := make([]string, 0)

	if !node.Ready || !node.Schedulable {
		return &v1.Prediction{
			NodeName:       node.Name,
			Probability:    0.0,
			Confidence:     1.0,
			DrainCandidate: false,
			Reasons:        []string{"Node is not ready or is unschedulable"},
			Timestamp:      timestamp,
		}
	}

	totalPods := len(pods)
	movablePods := 0
	unmovablePods := 0
	var unmovableReasons []string

	for _, pod := range pods {
		if pod.IsMovable {
			movablePods++
		} else {
			unmovablePods++
			reason := pod.UnmovableReason
			if reason == "" {
				reason = "non-movable workload constraints"
			}
			unmovableReasons = append(unmovableReasons, fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, reason))
		}
	}

	// 1. Hard blocker: unmovable workloads prevent consolidation
	if unmovablePods > 0 {
		return &v1.Prediction{
			NodeName:       node.Name,
			Probability:    0.0,
			Confidence:     0.95,
			DrainCandidate: false,
			Reasons: []string{
				fmt.Sprintf("Node hosts %d unmovable workload(s): %v", unmovablePods, unmovableReasons),
				"Consolidation blocked by workload placement constraints",
			},
			Timestamp: timestamp,
		}
	}

	// Calculate load ratios
	cpuLoad := 0.0
	memLoad := 0.0
	if node.Allocatable.CPU > 0 {
		cpuLoad = float64(node.Requested.CPU) / float64(node.Allocatable.CPU)
	}
	if node.Allocatable.Memory > 0 {
		memLoad = float64(node.Requested.Memory) / float64(node.Allocatable.Memory)
	}
	maxLoad := math.Max(cpuLoad, memLoad)

	// 2. Headroom & Distance to waterline
	utilDist := 0.0
	if maxLoad < e.candidateThreshold {
		utilDist = (e.candidateThreshold - maxLoad) / e.candidateThreshold
		reasons = append(reasons, fmt.Sprintf("Node utilization (CPU: %.1f%%, Mem: %.1f%%) is strictly below candidate threshold (%.1f%%)",
			cpuLoad*100, memLoad*100, e.candidateThreshold*100))
	} else {
		reasons = append(reasons, fmt.Sprintf("Node utilization (CPU: %.1f%%, Mem: %.1f%%) meets or exceeds active waterline threshold",
			cpuLoad*100, memLoad*100))
	}

	// 3. Replacement Capacity Feasibility
	// Remaining capacity in cluster excluding this node
	canAbsorb := clusterRemainingCPU >= node.Requested.CPU && clusterRemainingMem >= node.Requested.Memory
	replacementScore := 0.0
	if canAbsorb {
		replacementScore = 1.0
		reasons = append(reasons, fmt.Sprintf("Sufficient replacement capacity verified: cluster has %dm CPU and %dMB Mem available to absorb this node's %d pod(s)",
			clusterRemainingCPU, clusterRemainingMem/(1024*1024), totalPods))
	} else {
		reasons = append(reasons, fmt.Sprintf("Cluster lacks replacement capacity: needed %dm CPU / %dMB Mem, available %dm CPU / %dMB Mem",
			node.Requested.CPU, node.Requested.Memory/(1024*1024), clusterRemainingCPU, clusterRemainingMem/(1024*1024)))
	}

	// 4. Resource Fragmentation signal
	frag := math.Abs(cpuLoad - memLoad)
	if frag > 0.3 {
		reasons = append(reasons, fmt.Sprintf("High resource fragmentation detected (skew: %.2f)", frag))
	}

	// 5. Pod Mobility ratio
	mobRatio := 1.0
	if totalPods > 0 {
		mobRatio = float64(movablePods) / float64(totalPods)
	}

	// Deterministic probability calculation
	prob := (0.45 * utilDist) + (0.35 * replacementScore) + (0.15 * mobRatio) + (0.05 * frag)
	if !canAbsorb || maxLoad >= e.candidateThreshold {
		prob = math.Min(prob, 0.25) // Cap at low probability if not absorbable or above threshold
	}

	isCandidate := prob >= 0.60 && canAbsorb
	confidence := 0.85
	if totalPods == 0 {
		prob = 1.0
		isCandidate = true
		confidence = 0.99
		reasons = []string{"Node is completely empty (0 pods) and ready for immediate consolidation/deregistration"}
	}

	return &v1.Prediction{
		NodeName:       node.Name,
		Probability:    math.Round(prob*100) / 100,
		Confidence:     math.Round(confidence*100) / 100,
		DrainCandidate: isCandidate,
		Reasons:        reasons,
		Timestamp:      timestamp,
	}
}
