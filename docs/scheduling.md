# Kubernetes Scheduling Integration & Plugin Lifecycle

## Scheduling Framework Integration
The **Predictive Scheduler Plugin** (`PredictiveSchedulingScorer`) integrates directly into the upstream Kubernetes Scheduler Framework as a scoring extension.

```
Incoming Pod
     │
     ▼
┌────────────────────────────────────────────────────────┐
│ Filter Phase (Standard Upstream Plugins)               │
│ - NodeResourcesFit                                     │
│ - NodeAffinity & NodeSelector                          │
│ - TaintToleration                                      │
│ - PodTopologySpread                                    │
│ - VolumeBinding / PV Node Affinity                     │
└────────────────────────────┬───────────────────────────┘
                             │ (Feasible Nodes)
                             ▼
┌────────────────────────────────────────────────────────┐
│ PreScore Phase (Predictive Plugin)                     │
│ - Fetch node state from in-memory cache                │
└────────────────────────────┬───────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────┐
│ Score Phase (Predictive Plugin)                        │
│ - Safe Retained Nodes: 70 - 100 points                 │
│ - Drain Candidate Nodes: 0 - 15 points                 │
│ - Neutral / Unseen Nodes: 50 points                    │
└────────────────────────────┬───────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────┐
│ Normalize & Combine Scores                             │
│ - Map composite score to [0, 100]                      │
└────────────────────────────┬───────────────────────────┘
                             │
                             ▼
┌────────────────────────────────────────────────────────┐
│ Reserve, Permit & Bind (Standard Two-Phase Commit)     │
└────────────────────────────────────────────────────────┘
```

---

## Why Standard Scheduling Semantics Are Preserved
1. **Filtering is Never Bypassed**: The plugin only scores nodes that have already passed all scheduling filters. A pod will never be scheduled on a node that lacks memory or violates topology spread.
2. **Sub-Microsecond Latency**: The plugin does not query etcd or invoke external HTTP endpoints during the scheduling cycle. It performs an $O(1)$ memory lookup against the pre-computed `NodeScoreCache`, executing in approximately 90 nanoseconds per node candidate.
3. **No Stale Mutations**: Pod specs are not mutated with hardcoded node selectors. If cluster state changes, the scheduler naturally adapts during subsequent placement cycles.
