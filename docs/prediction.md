# Predictive Model & Explainability

## Design Principle: Transparent Determinism Over Opaque ML
Modern infrastructure operations require transparency, determinism, and auditability. Deploying black-box machine learning models inside core Kubernetes scheduling loops creates significant operational risks:
- Unpredictable failure modes during out-of-distribution traffic spikes.
- Inability for SREs to understand why a specific node was flagged for eviction.
- Heavy dependencies (Python runtime, GPU accelerators, high memory overhead).

The **Predictive Engine** uses a transparent, deterministic probabilistic model:
$$P(\text{node will be consolidated within window})$$

---

## Signal Pipeline
The model evaluates four key signals for each node:

1. **Waterline Utilization Distance ($S_{\text{util}}$)**:
   Measures how far below the candidate threshold (default 40%) the node currently operates:
   $$S_{\text{util}} = \max\left(0, \frac{\tau - \max(u_{\text{cpu}}, u_{\text{mem}})}{\tau}\right)$$

2. **Replacement Feasibility ($S_{\text{rep}}$)**:
   Binary and capacity verification: Can the safe nodes absorb all movable workloads from this candidate node without exceeding safety headroom?
   $$S_{\text{rep}} \in \{0, 1\}$$

3. **Workload Mobility ($S_{\text{mob}}$)**:
   Fraction of pods on the node that are safe to move (excludes DaemonSets, local host storage, standalone pods):
   $$S_{\text{mob}} = \frac{N_{\text{movable}}}{N_{\text{total}}}$$

4. **Resource Fragmentation ($S_{\text{frag}}$)**:
   Imbalance between CPU and memory load:
   $$S_{\text{frag}} = |u_{\text{cpu}} - u_{\text{mem}}|$$

---

## Prediction Output Format
Each prediction includes structured, human-readable explanations:

```json
{
  "nodeName": "worker-node-08",
  "probability": 0.85,
  "confidence": 0.85,
  "drainCandidate": true,
  "reasons": [
    "Node utilization (CPU: 10.0%, Mem: 15.0%) is strictly below candidate threshold (40.0%)",
    "Sufficient replacement capacity verified: cluster has 4000m CPU and 16384MB Mem available to absorb this node's 2 pod(s)",
    "All 2 pods on node are movable to safe nodes with confirmed replacement capacity"
  ],
  "timestamp": "2026-09-06T00:39:41Z"
}
```
