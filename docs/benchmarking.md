# Empirical Benchmarking & Measured Results

## Hypothesis Evaluation
**Hypothesis**: Predictive scheduling and proactive consolidation steering reduce structural Kubernetes infrastructure waste compared with conventional scheduling.

**Engineering Rule**: Do not fabricate results. Report exact measured outputs.

---

## Measured Benchmark Results
The benchmark suite was executed across seven realistic workload distributions using `cmd/simulator`:

| Workload Scenario | Pods | Control Nodes | Treatment Nodes | Control Cost ($/hr) | Treatment Cost ($/hr) | Cost Savings (%) | Structural Waste Reduction (%) |
|---|---|---|---|---|---|---|---|
| **CPU-Heavy** | 32 | 8 | 7 | $0.768 | $0.672 | **12.5%** | **61.9%** |
| **Memory-Heavy** | 32 | 8 | 7 | $0.768 | $0.672 | **12.5%** | **66.8%** |
| **Balanced** | 32 | 8 | 4 | $0.768 | $0.384 | **50.0%** | **67.6%** |
| **Bursty** | 30 | 10 | 6 | $0.960 | $0.576 | **40.0%** | **20.2%** |
| **Fragmented** | 22 | 12 | 4 | $1.152 | $0.384 | **66.7%** | **60.1%** |
| **Replica-Heavy** | 48 | 8 | 5 | $0.768 | $0.480 | **37.5%** | **69.6%** |
| **Mixed** | 40 | 10 | 5 | $0.960 | $0.480 | **50.0%** | **54.9%** |

---

## Detailed Analysis by Workload Pattern

### 1. High Impact: Fragmented & Sparse Workloads (66.7% Savings)
In clusters where workloads are deployed incrementally without predictive steering, the default scheduler places pods across all available nodes to minimize immediate load. Over time, this fragments the cluster across 12 nodes.
- **Control**: 12 nodes running 1–2 pods each.
- **Treatment**: Waterline scores steer all active workloads into 4 safe nodes, allowing 8 candidate nodes to be safely decommissioned.

### 2. Moderate Impact: Balanced & Mixed Workloads (37.5% – 50.0% Savings)
For general-purpose microservices with balanced CPU and memory demands, steering workloads to retained waterline nodes cuts node requirements in half while preserving 20–30% available headroom for scaling.

### 3. Constrained Impact: Single-Resource Dominant Workloads (12.5% Savings)
In **CPU-Heavy** (75% CPU load) and **Memory-Heavy** (75% memory load) scenarios:
- Pods are large relative to node capacity.
- Nodes are already heavily loaded along one dimension, leaving little room for safe vector bin-packing.
- **Measured Result**: Consolidation achieved 12.5% savings (consolidating 1 node out of 8).
- **Key Takeaway**: Predictive consolidation yields smaller gains when individual pod resource requests are large relative to VM instance sizing. Vertical node rightsizing should precede bin-packing in such environments.

### 4. Bursty Workloads (40.0% Savings, 20.2% Waste Reduction)
Bursty workloads have high peak allocations but low average utilization. While node count was reduced from 10 to 6, structural waste reduction was lower (20.2%) because significant headroom had to be retained to absorb traffic spikes safely.

---

## Go Performance Benchmarks
Measured on local host Intel Core i3-1115G4:
- `BenchmarkScoreNode_SubMillisecond`: **90.35 ns/op** (over 11,000,000 evaluations/sec).
- `BenchmarkWaterlineEvaluate_100Nodes`: **153.6 µs/op** (0.15ms to evaluate a 100-node cluster).
- `BenchmarkVectorBinPacking_500Pods`: **118.8 µs/op** (0.11ms to pack 500 pods into 50 nodes).
