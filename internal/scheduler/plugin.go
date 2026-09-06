package scheduler

import (
	"context"
	"fmt"
	"math"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/metrics"
)

const (
	// PluginName is the official registered name of the predictive scoring plugin.
	PluginName = "PredictiveSchedulingScorer"

	// Framework score bounds in Kubernetes Scheduler Framework
	MinNodeScore = int64(0)
	MaxNodeScore = int64(100)
)

// Plugin implements the scoring extension points for Kubernetes Scheduler Framework.
type Plugin struct {
	cache       NodeScoreCache
	weight      int64
	minDrainCap int64
}

// NewPlugin constructs the predictive scoring plugin.
func NewPlugin(cache NodeScoreCache, weight int64) *Plugin {
	if weight <= 0 {
		weight = 1
	}
	if cache == nil {
		cache = NewInMemoryScoreCache()
	}
	return &Plugin{
		cache:  cache,
		weight: weight,
	}
}

// Name returns the identifier of the plugin.
func (p *Plugin) Name() string {
	return PluginName
}

// ScoreNode returns a score between 0 and 100 for a candidate node given the pod.
// Safe nodes receive high scores, while drain candidates are penalized.
func (p *Plugin) ScoreNode(ctx context.Context, pod *v1.PodInfo, nodeName string) (int64, error) {
	safetyScore, role, found := p.cache.GetNodeScore(nodeName)

	var score int64
	if !found {
		// Uncached node defaults to neutral score
		score = 50
	} else {
		switch role {
		case v1.NodeRoleSafe:
			// Reward safe nodes: scaled between 70 and 100
			score = int64(math.Round(70.0 + (safetyScore * 0.30)))
			if score > MaxNodeScore {
				score = MaxNodeScore
			}
		case v1.NodeRoleCandidate:
			// Drain candidates are penalized to prevent new workloads from landing on them
			// Scaled between 0 and 15
			score = int64(math.Round(safetyScore * 0.15))
			if score < MinNodeScore {
				score = MinNodeScore
			}
		default: // Neutral
			score = int64(math.Round(safetyScore))
		}
	}

	metrics.SchedulingDecisionsTotal.WithLabelValues(nodeName, string(role)).Inc()
	return score, nil
}

// NormalizeScore scales raw scores across all candidate nodes to [0, 100].
func (p *Plugin) NormalizeScore(scores map[string]int64) map[string]int64 {
	if len(scores) == 0 {
		return scores
	}

	normalized := make(map[string]int64, len(scores))
	for node, s := range scores {
		if s < MinNodeScore {
			s = MinNodeScore
		}
		if s > MaxNodeScore {
			s = MaxNodeScore
		}
		normalized[node] = s
	}

	return normalized
}

// EvaluateCandidateSet scores and ranks a set of candidate nodes for a pod.
func (p *Plugin) EvaluateCandidateSet(ctx context.Context, pod *v1.PodInfo, candidateNodeNames []string) (string, map[string]int64, error) {
	if len(candidateNodeNames) == 0 {
		return "", nil, fmt.Errorf("no candidate nodes provided")
	}

	rawScores := make(map[string]int64, len(candidateNodeNames))
	for _, name := range candidateNodeNames {
		s, err := p.ScoreNode(ctx, pod, name)
		if err != nil {
			return "", nil, err
		}
		rawScores[name] = s
	}

	normalized := p.NormalizeScore(rawScores)

	// Pick highest scoring node
	bestNode := ""
	bestScore := int64(-1)
	for _, name := range candidateNodeNames {
		s := normalized[name]
		if s > bestScore {
			bestScore = s
			bestNode = name
		}
	}

	return bestNode, normalized, nil
}
