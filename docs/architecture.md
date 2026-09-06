# System Architecture

## Overview
The **Kubernetes Predictive Scheduling & Cost Optimization Engine** is a proactive infrastructure optimization platform designed to eliminate structural cloud waste. Rather than relying solely on reactive deschedulers, the engine predicts which nodes are candidates for future consolidation and steers incoming workloads toward nodes destined to remain active.

---

## Core Components

### 1. Predictive Scheduler Plugin (`cmd/scheduler-plugin`, `internal/scheduler`)
- A custom Kubernetes scheduler binary running under `schedulerName: predictive-scheduler`.
- Registers the `PredictiveSchedulingScorer` plugin into the Kubernetes Scheduler Framework.
- Performs sub-microsecond score lookups from the in-memory cache:
  - **Safe Waterline Nodes**: Scored between 70 and 100.
  - **Drain Candidate Nodes**: Penalized to scores between 0 and 15.
  - **Neutral Nodes**: Baseline score of 50.
- Guarantees complete compliance with Kubernetes predicates: taints, tolerations, volume zone binding, topology spread, and pod anti-affinity.

### 2. Predictive Admission Webhook (`cmd/webhook`, `internal/admission`)
- Acts as the experiment router and classification switchboard.
- Intercepts Pod creation requests.
- Inspects namespace labels (`predictive-scheduler/group: treatment` vs `control`).
- For treatment pods: Mutates `spec.schedulerName` to `predictive-scheduler`.
- For control pods: Retains `spec.schedulerName: default-scheduler`.
- **Safety First**: Implements `failurePolicy: Ignore`. If the webhook crashes or times out, the pod is admitted with normal Kubernetes scheduling.

### 3. Simulation & Waterline Engine (`cmd/simulator`, `internal/simulation`, `internal/waterline`)
- Runs asynchronously outside the pod admission critical path.
- Periodically ingests a point-in-time snapshot of the cluster: nodes, pods, requested resources, used metrics, and mobility constraints.
- Calculates multi-factor node scores:
  - $\text{DrainScore}$: Utilization headroom, fragmentation, workload mobility, disruption risk.
  - $\text{SafetyScore} = 100 - \text{DrainScore}$.
- Runs multi-dimensional vector bin-packing simulations (Best-Fit Decreasing) to verify whether workloads on candidate nodes can be absorbed by safe nodes.
- Updates the in-memory score cache read by the scheduler plugin.

### 4. Eviction Controller (`cmd/controller`, `internal/eviction`)
- A safety-guarded background reconciler responsible for gentle, rate-limited consolidation.
- Disabled by default (`eviction.enabled: false`) to guarantee zero surprise disruptions.
- Enforces strict safety gates before evicting:
  1. Pod must NOT belong to protected namespaces (`kube-system`, `monitoring`, etc.).
  2. Pod must NOT be an unmovable DaemonSet or unmanaged standalone pod.
  3. PodDisruptionBudget must permit disruptions (`DisruptionsAllowed > 0`).
  4. Safe nodes must have verified replacement headroom (maintaining at least 20% available capacity).
  5. Rate limiter enforces maximum pods per hour and per-node cooldown windows.

### 5. FinOps & Waste Engine (`internal/cost`)
- Decomposes cluster waste into multi-dimensional components:
  - **CPU Waste %**: $(Requested - Used) / Requested \times 100$
  - **Memory Waste %**: $(Requested - Used) / Requested \times 100$
  - **Structural Waste %**: Unallocated provisioned capacity + unused requested capacity.
- Maps AWS EC2 instance types to hourly costs and calculates real deltas between Control and Treatment.

### 6. Storage & Archival (`internal/storage`)
- Stores sanitized cluster snapshots and prediction reports to Amazon S3 (authenticated via IRSA) or local disk.
- Automatically strips sensitive tokens, secrets, environment variables, and unneeded metadata.
