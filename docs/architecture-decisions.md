# Architecture Decisions Record (ADR)

## ADR 001: Architectural Correction for Kubernetes Predictive Scheduling

### Status
Accepted & Implemented

### Context
The original proposed architecture suggested placing a Predictive Admission Webhook directly on the pod admission path to execute a Waterline simulation engine and make scheduling decisions:

```
Pod CREATE -> API Server -> Mutating Admission Webhook -> Simulation / Waterline Engine -> Decision -> Scheduler -> Node
```

### Problem Analysis
During our architectural audit, we evaluated whether a mutating admission webhook is technically capable or appropriate for pod placement decisions:

1. **Bypassing the Scheduler**:
   - If a mutating webhook assigns a node directly by setting `spec.nodeName`, it **completely bypasses the Kubernetes Scheduler**.
   - Bypassing the scheduler skips all core predicates, filter plugins, and constraints:
     - Resource checks (CPU, Memory, Ephemeral Storage)
     - Node taints and pod tolerations
     - Pod topology spread constraints
     - Inter-pod affinity and anti-affinity
     - Persistent volume node affinity and zone constraints
   - If the selected node cannot run the pod, the kubelet rejects it, causing `FailedCreate` or `OutOfmemory` crashes.

2. **Admission Latency Explosion**:
   - Mutating admission webhooks run synchronously on the Kubernetes API server write path.
   - Running cluster-wide vector bin-packing simulations synchronously on every pod admission introduces hundreds of milliseconds of latency, risking webhook timeouts (`timeoutSeconds: 10s`) and degrading control plane throughput.

3. **Concurrency and Race Conditions**:
   - Multiple pods admitted concurrently are invisible to one another at admission time because admission webhooks lack the two-phase reservation cycle (`Reserve` and `Permit`) of the Kubernetes scheduler. This causes a stampeding herd onto the same "safe" node.

4. **Hardcoded Persisted Mutations**:
   - If the webhook sets `spec.affinity` or `spec.nodeSelector`, that mutation is permanently persisted in etcd. If cluster conditions change and the pod must be rescheduled, the stale affinity remains locked in the pod spec.

### Architectural Decision
We replaced the synchronous admission webhook placement model with a **decoupled cooperative architecture**:

```
                                  +-------------------------------------------------------+
                                  |                 Kubernetes API Server                 |
                                  +---------------------------+---------------------------+
                                                              |
                                               +--------------+--------------+
                                               |                             |
                                      [Pod CREATE (Control)]       [Pod CREATE (Treatment)]
                                               |                             |
                                               v                             v
                                     default-scheduler          predictive-admission-webhook
                                    (Standard Bin-pack)         (Injects schedulerName:
                                               |                 predictive-scheduler)
                                               |                             |
                                               |                             v
                                               |                 predictive-scheduler
                                               |                 (Scheduler Plugin)
                                               |                             |
                                               |            Reads Cache      v
                                               |       +----------------[Score / Filter]
                                               |       |                     |
                                               v       v                     v
                                      +--------------------+        +--------------------+
                                      |    Worker Nodes    |        |    Worker Nodes    |
                                      | (Control Group)    |        | (Treatment Group)  |
                                      +--------------------+        +--------------------+
```

1. **Kubernetes Scheduler Framework Plugin (`predictive-scheduler`)**:
   - Implements `Score` and `PreScore` extension points.
   - Evaluates candidate nodes against cached Waterline scores in microseconds (~90ns in benchmarks).
   - Preserves 100% of standard Kubernetes scheduling semantics (taints, tolerations, volume binding, topology spread).

2. **Mutating Admission Webhook as Experiment Router**:
   - The webhook is retained strictly for **fail-safe experiment routing**.
   - If a pod belongs to a `treatment` namespace, it mutates `spec.schedulerName` to `predictive-scheduler`.
   - If `control`, it preserves `default-scheduler`.
   - Configured with `failurePolicy: Ignore` and a 3-second timeout: if the webhook fails, the pod seamlessly falls back to `default-scheduler` without cluster disruption.

3. **Asynchronous Background Simulation & Waterline Engine**:
   - Decoupled from the synchronous pod admission path.
   - Runs on periodic intervals (e.g. 15m) or cluster delta triggers to calculate waterline levels, drain candidate probabilities, and update the in-memory score cache.

---

## ADR 002: In-Cluster Controller vs EventBridge + Lambda

### Status
Accepted & Implemented

### Context
The initial proposal suggested triggering periodic simulations using AWS EventBridge and an external AWS Lambda function.

### Problem Analysis
- Driving core Kubernetes scheduling cycles from an external Lambda requires opening ingress into the private EKS API server (or deploying complex VPC endpoints / peering).
- It creates an unnecessary external cloud dependency for an internal cluster scheduling loop.
- Network partitions or AWS Lambda throttling could stall cluster optimization.

### Decision
- The primary, resilient scheduling and simulation trigger is an **in-cluster controller reconciler loop** with native leader election.
- AWS EventBridge / Lambda can optionally be connected as an external audit or FinOps reporting trigger, but cluster availability never depends on it.
