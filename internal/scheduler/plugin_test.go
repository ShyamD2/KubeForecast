package scheduler

import (
	"context"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
)

func TestPredictivePlugin_ScoreNode(t *testing.T) {
	cache := NewInMemoryScoreCache()
	cache.UpdateScores(&v1.WaterlineReport{
		SafeNodes: []string{"node-safe"},
		DrainCandidates: []v1.DrainCandidateInfo{
			{NodeName: "node-candidate", SafetyScore: 15.0, DrainScore: 85.0},
		},
		NeutralNodes: []string{"node-neutral"},
	}, map[string]*v1.NodeInfo{
		"node-safe": {SafetyScore: 90.0, DrainScore: 10.0},
	})

	plugin := NewPlugin(cache, 1)
	ctx := context.Background()
	pod := &v1.PodInfo{Name: "test-pod"}

	safeScore, err := plugin.ScoreNode(ctx, pod, "node-safe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	candScore, err := plugin.ScoreNode(ctx, pod, "node-candidate")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	neutralScore, err := plugin.ScoreNode(ctx, pod, "node-neutral")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if safeScore < 70 {
		t.Errorf("expected safe score >= 70, got %d", safeScore)
	}
	if candScore > 15 {
		t.Errorf("expected candidate score <= 15, got %d", candScore)
	}
	if safeScore <= candScore {
		t.Errorf("safe node (%d) must score substantially higher than candidate node (%d)", safeScore, candScore)
	}
	if neutralScore != 50 {
		t.Errorf("expected neutral node score 50, got %d", neutralScore)
	}
}

func TestPredictivePlugin_EvaluateCandidateSet(t *testing.T) {
	cache := NewInMemoryScoreCache()
	cache.UpdateScores(&v1.WaterlineReport{
		SafeNodes: []string{"node-safe-1", "node-safe-2"},
		DrainCandidates: []v1.DrainCandidateInfo{
			{NodeName: "node-cand-1", SafetyScore: 10.0},
		},
	}, map[string]*v1.NodeInfo{
		"node-safe-1": {SafetyScore: 95.0},
		"node-safe-2": {SafetyScore: 75.0},
	})

	plugin := NewPlugin(cache, 1)
	ctx := context.Background()
	pod := &v1.PodInfo{Name: "test-pod"}

	best, scores, err := plugin.EvaluateCandidateSet(ctx, pod, []string{"node-safe-1", "node-safe-2", "node-cand-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if best != "node-safe-1" {
		t.Errorf("expected node-safe-1 as best candidate, got: %s (scores: %v)", best, scores)
	}
}
