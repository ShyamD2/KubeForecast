package scheduler

import (
	"sync"
	"time"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

// NodeScoreCache provides lock-free or low-contention thread-safe storage
// for node Waterline scores, roles, and candidate statuses.
type NodeScoreCache interface {
	GetNodeScore(nodeName string) (safetyScore float64, role v1.NodeRole, found bool)
	UpdateScores(report *v1.WaterlineReport, nodes map[string]*v1.NodeInfo)
	GetAllScores() map[string]CachedNodeScore
}

type CachedNodeScore struct {
	NodeName    string
	SafetyScore float64
	DrainScore  float64
	Role        v1.NodeRole
	UpdatedAt   time.Time
}

type inMemoryScoreCache struct {
	mu     sync.RWMutex
	scores map[string]CachedNodeScore
}

// NewInMemoryScoreCache initializes a new thread-safe score cache.
func NewInMemoryScoreCache() NodeScoreCache {
	return &inMemoryScoreCache{
		scores: make(map[string]CachedNodeScore),
	}
}

func (c *inMemoryScoreCache) GetNodeScore(nodeName string) (float64, v1.NodeRole, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.scores[nodeName]
	if !ok {
		return 50.0, v1.NodeRoleNeutral, false // default neutral baseline
	}
	return entry.SafetyScore, entry.Role, true
}

func (c *inMemoryScoreCache) UpdateScores(report *v1.WaterlineReport, nodes map[string]*v1.NodeInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()

	// Update safe nodes
	for _, name := range report.SafeNodes {
		ss := 80.0
		ds := 20.0
		if n, ok := nodes[name]; ok {
			ss = n.SafetyScore
			ds = n.DrainScore
		}
		c.scores[name] = CachedNodeScore{
			NodeName:    name,
			SafetyScore: ss,
			DrainScore:  ds,
			Role:        v1.NodeRoleSafe,
			UpdatedAt:   now,
		}
	}

	// Update candidate nodes
	for _, cand := range report.DrainCandidates {
		c.scores[cand.NodeName] = CachedNodeScore{
			NodeName:    cand.NodeName,
			SafetyScore: cand.SafetyScore,
			DrainScore:  cand.DrainScore,
			Role:        v1.NodeRoleCandidate,
			UpdatedAt:   now,
		}
	}

	// Update neutral nodes
	for _, name := range report.NeutralNodes {
		c.scores[name] = CachedNodeScore{
			NodeName:    name,
			SafetyScore: 50.0,
			DrainScore:  50.0,
			Role:        v1.NodeRoleNeutral,
			UpdatedAt:   now,
		}
	}
}

func (c *inMemoryScoreCache) GetAllScores() map[string]CachedNodeScore {
	c.mu.RLock()
	defer c.mu.RUnlock()
	copyMap := make(map[string]CachedNodeScore, len(c.scores))
	for k, v := range c.scores {
		copyMap[k] = v
	}
	return copyMap
}
