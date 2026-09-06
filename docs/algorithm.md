# The Waterline Algorithm & Scoring Mechanism

## Core Concept
In a typical Kubernetes cluster, workloads are spread across nodes by default bin-packing or least-requested algorithms. This organic distribution often results in "stranding" small pockets of CPU and memory across dozens of nodes—none of which are empty enough to drain, yet all of which are paid for in full.

The **Waterline Algorithm** establishes a dynamic threshold $\Omega$ separating the cluster into two distinct zones:
1. **Safe Retained Zone (Above Waterline)**: High-density nodes that are designated to remain active. Workloads on these nodes are protected.
2. **Candidate Consolidation Zone (Below Waterline)**: Underutilized nodes whose workloads can be packed into the remaining headroom of the Retained Zone.

---

## Mathematical Scoring Formulas

### 1. Load Ratios
For candidate node $i$ with allocatable resources $A_{\text{cpu}}, A_{\text{mem}}$ and requested resources $R_{\text{cpu}}, R_{\text{mem}}$:
$$u_{\text{cpu}} = \min\left(1.0, \frac{R_{\text{cpu}}}{A_{\text{cpu}}}\right), \quad u_{\text{mem}} = \min\left(1.0, \frac{R_{\text{mem}}}{A_{\text{mem}}}\right)$$

Headroom available:
$$H_{\text{cpu}} = 1.0 - u_{\text{cpu}}, \quad H_{\text{mem}} = 1.0 - u_{\text{mem}}$$

### 2. Fragmentation Index
$$F = |u_{\text{cpu}} - u_{\text{mem}}|$$
A large divergence between CPU and memory requests creates fragmentation, making the remaining capacity difficult to fill organically.

### 3. Workload Mobility & Disruption Risk
Let $N_{\text{total}}$ be the total pods on node $i$:
$$M = \frac{N_{\text{movable}}}{\max(1, N_{\text{total}})}, \quad D = \frac{N_{\text{unmovable}} + N_{\text{pdb\_protected}}}{\max(1, N_{\text{total}})}$$

- Workloads managed by DaemonSets, pods using local host storage, or standalone pods without controller owners are classified as **unmovable** ($M \to 0, D \to 1$).

### 4. Consolidation Potential
$$C_{\text{pot}} = \left(\frac{H_{\text{cpu}} + H_{\text{mem}}}{2}\right) \times M$$

### 5. Multi-Factor DrainScore
$$\text{RawDrain} = w_{\text{cpu}} H_{\text{cpu}} + w_{\text{mem}} H_{\text{mem}} + w_{\text{consolidation}} C_{\text{pot}} + 0.05 F - w_{\text{disruption}} D$$

$$\text{DrainScore} = \text{clamp}(\text{RawDrain} \times 100, 0, 100)$$
$$\text{SafetyScore} = 100.0 - \text{DrainScore}$$

---

## Configurable Scoring Weights
The scoring formula is entirely configurable:

```yaml
waterline:
  cpuWeight: 0.30           # Weight assigned to CPU headroom
  memoryWeight: 0.30        # Weight assigned to Memory headroom
  consolidationWeight: 0.20 # Weight on movable consolidation potential
  disruptionWeight: 0.15    # Penalty for PDB / unmovable workloads
  historicalWeight: 0.05    # Historical stability weighting
  targetUtilizationThreshold: 0.75 # Target waterline level (75%)
  candidateDrainThreshold: 0.40    # Candidates must be below 40% load
  minNodesFloor: 2                 # Absolute minimum active nodes
```

---

## Placement Score for Incoming Pods
When an incoming pod $p$ is evaluated by the scheduler:
$$\text{PlacementScore} = 0.75 \times \text{SafetyScore} + 0.25 \times \text{FitScore}$$
If the candidate node is flagged as `NodeRoleCandidate`, its placement score is penalized by 90% ($\times 0.10$), actively steering new pods away from drain targets.
