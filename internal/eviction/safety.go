package eviction

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/consolidation"
)

// SafetyValidator checks whether a pod is eligible for safe, non-disruptive eviction.
type SafetyValidator interface {
	ValidatePod(pod *v1.PodInfo) (safe bool, reason string)
	ValidateNodeCooldown(nodeName string) (safe bool, waitRemaining time.Duration)
	ValidateClusterCapacity(pod *v1.PodInfo, safeNodes []*v1.NodeInfo, minHeadroomPct float64) (destinationNode string, ok bool)
}

type safetyValidator struct {
	mu           sync.RWMutex
	nodeCooldown map[string]time.Time
	cooldown     time.Duration
}

// NewSafetyValidator constructs a SafetyValidator with configured cooldown duration.
func NewSafetyValidator(cooldown time.Duration) SafetyValidator {
	if cooldown <= 0 {
		cooldown = 15 * time.Minute
	}
	return &safetyValidator{
		nodeCooldown: make(map[string]time.Time),
		cooldown:     cooldown,
	}
}

var protectedNamespaces = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"kube-node-lease": true,
	"monitoring":      true,
}

// ValidatePod verifies system protections, unmovable flags, and PDB constraints.
func (v *safetyValidator) ValidatePod(pod *v1.PodInfo) (bool, string) {
	if pod == nil {
		return false, "nil pod"
	}

	// 1. Protected namespaces
	if protectedNamespaces[strings.ToLower(pod.Namespace)] {
		return false, fmt.Sprintf("pod belongs to protected system namespace %q", pod.Namespace)
	}

	// 2. Mobility flag
	if !pod.IsMovable {
		reason := pod.UnmovableReason
		if reason == "" {
			reason = "flagged unmovable (e.g. DaemonSet, local storage, host port, non-replicated)"
		}
		return false, reason
	}

	// 3. PodDisruptionBudget check
	if pod.HasPDB && pod.DisruptionsAllowed <= 0 {
		return false, "pod disruption budget allows 0 disruptions currently"
	}

	return true, ""
}

// ValidateNodeCooldown ensures rate-limiting per node.
func (v *safetyValidator) ValidateNodeCooldown(nodeName string) (bool, time.Duration) {
	v.mu.RLock()
	lastEviction, exists := v.nodeCooldown[nodeName]
	v.mu.RUnlock()

	if !exists {
		return true, 0
	}

	elapsed := time.Since(lastEviction)
	if elapsed < v.cooldown {
		return false, v.cooldown - elapsed
	}

	return true, 0
}

// RecordEviction updates the node cooldown timestamp.
func (v *safetyValidator) RecordEviction(nodeName string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.nodeCooldown[nodeName] = time.Now()
}

// ValidateClusterCapacity verifies that safe nodes have sufficient replacement headroom.
func (v *safetyValidator) ValidateClusterCapacity(pod *v1.PodInfo, safeNodes []*v1.NodeInfo, minHeadroomPct float64) (string, bool) {
	// Attempt Best-Fit Decreasing packing into safe nodes
	res := consolidation.PackPodsBestFitDecreasing([]*v1.PodInfo{pod}, safeNodes, 110)
	if !res.Success {
		return "", false
	}

	destNode := res.Assignments[pod.Name]
	if destNode == "" {
		return "", false
	}

	// Ensure the chosen node maintains minHeadroomPct
	for _, n := range safeNodes {
		if n.Name == destNode {
			projectedUsedCPU := n.Requested.CPU + pod.Requested.CPU
			headroomCPU := 1.0 - (float64(projectedUsedCPU) / float64(n.Allocatable.CPU))
			if headroomCPU < minHeadroomPct {
				return "", false
			}
			return destNode, true
		}
	}

	return destNode, true
}

// Controller coordinates gentle, rate-limited eviction of consolidation candidate workloads.
type Controller struct {
	config     v1.EvictionConfig
	validator  SafetyValidator
	hourlyPool int
	poolReset  time.Time
	mu         sync.Mutex
}

// NewController creates an eviction controller instance.
func NewController(config v1.EvictionConfig) *Controller {
	return &Controller{
		config:     config,
		validator:  NewSafetyValidator(config.Cooldown),
		hourlyPool: config.MaxPodsPerHour,
		poolReset:  time.Now().Add(1 * time.Hour),
	}
}

// PlanEvictions generates a dry-run validated eviction plan for candidate nodes.
func (c *Controller) PlanEvictions(ctx context.Context, state *v1.ClusterState, candidates []v1.DrainCandidateInfo, safeNodeNames []string) (*v1.EvictionPlan, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset hourly rate limiter pool
	if time.Now().After(c.poolReset) {
		c.hourlyPool = c.config.MaxPodsPerHour
		c.poolReset = time.Now().Add(1 * time.Hour)
	}

	plan := &v1.EvictionPlan{
		Timestamp: time.Now().UTC(),
		Targets:   make([]v1.EvictionTarget, 0),
	}

	if len(candidates) == 0 {
		plan.Feasible = true
		return plan, nil
	}

	// Extract safe nodes
	safeNodeMap := make(map[string]*v1.NodeInfo)
	safeNodes := make([]*v1.NodeInfo, 0)
	for _, name := range safeNodeNames {
		if n, ok := state.Nodes[name]; ok {
			safeNodeMap[name] = n
			safeNodes = append(safeNodes, n)
		}
	}

	// Index pods by node
	nodePods := make(map[string][]*v1.PodInfo)
	for _, p := range state.Pods {
		if p.NodeName != "" {
			nodePods[p.NodeName] = append(nodePods[p.NodeName], p)
		}
	}

	// Iterate through candidates up to rate limit
	for _, cand := range candidates {
		// Check node cooldown
		if safe, rem := c.validator.ValidateNodeCooldown(cand.NodeName); !safe {
			plan.SafetyErrors = append(plan.SafetyErrors, fmt.Sprintf("node %s is in cooldown (%v remaining)", cand.NodeName, rem.Round(time.Second)))
			continue
		}

		pods := nodePods[cand.NodeName]
		for _, pod := range pods {
			if c.hourlyPool <= 0 {
				plan.SafetyErrors = append(plan.SafetyErrors, "hourly eviction budget reached")
				break
			}

			// Validate pod eligibility
			if ok, reason := c.validator.ValidatePod(pod); !ok {
				plan.SafetyErrors = append(plan.SafetyErrors, fmt.Sprintf("pod %s/%s cannot be evicted: %s", pod.Namespace, pod.Name, reason))
				continue
			}

			// Validate destination capacity
			destNode, ok := c.validator.ValidateClusterCapacity(pod, safeNodes, c.config.MinAvailableCapacityPct)
			if !ok {
				plan.SafetyErrors = append(plan.SafetyErrors, fmt.Sprintf("no safe node has capacity with >= %.0f%% headroom for pod %s/%s", c.config.MinAvailableCapacityPct*100, pod.Namespace, pod.Name))
				continue
			}

			plan.CandidateNode = cand.NodeName
			plan.Targets = append(plan.Targets, v1.EvictionTarget{
				Namespace:       pod.Namespace,
				Name:            pod.Name,
				NodeName:        cand.NodeName,
				DestinationNode: destNode,
				Requested:       pod.Requested,
				Reason:          fmt.Sprintf("Predicted drain consolidation to safe node %s", destNode),
			})

			plan.TotalReclaimableCPU += pod.Requested.CPU
			plan.TotalReclaimableMem += pod.Requested.Memory

			// Enforce max concurrent
			if len(plan.Targets) >= c.config.MaxConcurrentEvictions {
				break
			}
		}

		if len(plan.Targets) > 0 {
			break // Only process one candidate node per cycle for bounded disruption
		}
	}

	plan.Feasible = len(plan.Targets) > 0
	return plan, nil
}
