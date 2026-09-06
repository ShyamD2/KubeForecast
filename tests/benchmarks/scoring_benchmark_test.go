package benchmarks

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/kubeforecast/kubernetes-predictive-scheduler/api/v1"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/consolidation"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/scheduler"
	"github.com/kubeforecast/kubernetes-predictive-scheduler/internal/waterline"
)

func BenchmarkScoreNode_SubMillisecond(b *testing.B) {
	cache := scheduler.NewInMemoryScoreCache()
	cache.UpdateScores(&v1.WaterlineReport{
		SafeNodes: []string{"node-safe"},
		DrainCandidates: []v1.DrainCandidateInfo{
			{NodeName: "node-candidate", SafetyScore: 10.0},
		},
	}, map[string]*v1.NodeInfo{
		"node-safe": {SafetyScore: 90.0},
	})

	plugin := scheduler.NewPlugin(cache, 1)
	ctx := context.Background()
	pod := &v1.PodInfo{Name: "bench-pod"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = plugin.ScoreNode(ctx, pod, "node-safe")
	}
}

func BenchmarkWaterlineEvaluate_100Nodes(b *testing.B) {
	cfg := v1.DefaultWaterlineConfig()
	pricing := v1.DefaultPricingConfig()
	engine := waterline.NewEngine(cfg, pricing)

	numNodes := 100
	nodes := make(map[string]*v1.NodeInfo, numNodes)
	pods := make([]*v1.PodInfo, 0, numNodes*5)

	for i := 0; i < numNodes; i++ {
		name := fmt.Sprintf("node-%03d", i)
		nodes[name] = &v1.NodeInfo{
			Name:         name,
			InstanceType: "m5.large",
			Allocatable:  v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:    v1.ResourceQuantities{CPU: int64(1000 + (i * 20)), Memory: int64(4000 + (i * 50))},
			Ready:        true,
			Schedulable:  true,
		}
		for p := 0; p < 5; p++ {
			pods = append(pods, &v1.PodInfo{
				Name:      fmt.Sprintf("%s-pod-%d", name, p),
				NodeName:  name,
				Requested: v1.ResourceQuantities{CPU: 200, Memory: 800},
				IsMovable: true,
			})
		}
	}

	state := &v1.ClusterState{
		Nodes: nodes,
		Pods:  pods,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Evaluate(state)
	}
}

func BenchmarkVectorBinPacking_500Pods(b *testing.B) {
	nodes := make([]*v1.NodeInfo, 50)
	for i := 0; i < 50; i++ {
		nodes[i] = &v1.NodeInfo{
			Name:        fmt.Sprintf("node-%d", i),
			Allocatable: v1.ResourceQuantities{CPU: 4000, Memory: 16384},
			Requested:   v1.ResourceQuantities{CPU: 1000, Memory: 4096},
		}
	}

	pods := make([]*v1.PodInfo, 500)
	for i := 0; i < 500; i++ {
		pods[i] = &v1.PodInfo{
			Name:      fmt.Sprintf("pod-%d", i),
			Requested: v1.ResourceQuantities{CPU: 200, Memory: 512},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = consolidation.PackPodsBestFitDecreasing(pods, nodes, 110)
	}
}
