# Live AWS EKS Soak Test - Continuous Telemetry Status

**Execution Window**: Live until 6:00 PM IST  
**Last Updated**: 2026-09-06T13:58:48.557576  
**Total Samples Collected**: 3  
**AWS Cluster**: `dev-predictive-eks` (3 x `c7i-flex.large`)

---

### Latest Sampling Snapshot (Cycle #3)

| Metric | Control (Default Scheduler) | Treatment (Predictive Scheduler) |
| :--- | :---: | :---: |
| **Total Pods Active** | 4 | 4 |
| **Nodes Occupied** | 2 of 3 | 1 of 3 |
| **Nodes Eligible for Drain/Consolidation** | 1 | 2 |
| **Fragmentation Score** | 0.67 (High) | 0.33 (Low / Packed) |
| **Simulated Cost Waste Reduction** | Baseline (0%) | **50.0%** |

### Per-Node Distribution
- **Control Distribution**: `{"ip-10-10-10-77.ec2.internal": 2, "ip-10-10-11-14.ec2.internal": 2}`
- **Treatment Distribution**: `{"ip-10-10-10-77.ec2.internal": 4}`

*(Continuous samples are appended to `experiments/results/live_soak_telemetry.jsonl`)*
