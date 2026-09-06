# Failure Testing & Chaos Engineering

## Resilience Philosophy
Workload availability and cluster stability take precedence over optimization. Under no circumstances should an optimization engine induce a service outage.

---

## Validated Failure Modes

### 1. Simulator Unavailable
- **Failure Condition**: The simulation daemon crashes or is partitioned from the API server.
- **Observed Behavior**: The scheduler plugin falls back to neutral baseline scores (50 points) for all candidate nodes. Standard Kubernetes scheduling continues without delay or errors (`TestFailure_SimulatorUnavailable` PASS).

### 2. Webhook Unavailable or Malformed Request
- **Failure Condition**: The admission webhook crashes, experiences TLS certificate expiration, or receives corrupt JSON.
- **Observed Behavior**: The webhook fails open (`failurePolicy: Ignore`). Unprocessed pods default to `default-scheduler` without failing admission (`TestFailure_WebhookUnavailableFailSafe` PASS).

### 3. S3 Snapshot Storage Unavailable
- **Failure Condition**: AWS S3 returns 503 Service Unavailable or IAM credentials expire.
- **Observed Behavior**: The simulation engine logs a warning, falls back to local disk snapshotting, and continues serving in-memory score updates to the scheduler plugin (`TestFailure_S3UnavailableGracefulFallback` PASS).

### 4. Prometheus Metrics Scraper Offline
- **Failure Condition**: Prometheus server is unreachable.
- **Observed Behavior**: Prometheus collectors update internal memory counters without blocking HTTP request execution.

### 5. PodDisruptionBudget (PDB) Blocks Eviction
- **Failure Condition**: A candidate node hosts a pod protected by a PDB where `DisruptionsAllowed == 0`.
- **Observed Behavior**: The safety validator immediately flags the PDB violation and aborts eviction planning for that workload (`TestFailure_PDBBlocksEviction` PASS).

### 6. Sudden Workload Spike (Capacity Starvation)
- **Failure Condition**: A sudden burst in traffic pushes safe retained nodes above 80% utilization.
- **Observed Behavior**: The safety validator enforces the `minAvailableCapacityPct: 0.20` headroom threshold. Because destination nodes lack 20% free headroom, all planned consolidations are suppressed until capacity recovers (`TestFailure_SuddenWorkloadSpikeYieldsToAvailability` PASS).
