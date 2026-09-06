# Observability & Metrics Specification

## Metric Catalog
The engine exports standard Prometheus metrics on port `8080` (controller and webhook) and port `10259` (scheduler plugin):

| Metric Name | Type | Description |
|---|---|---|
| `predictive_scheduler_webhook_requests_total` | Counter | Total admission webhook evaluation requests |
| `predictive_scheduler_webhook_latency_seconds` | Histogram | Latency distribution of admission evaluations |
| `predictive_scheduler_predictions_total` | Counter | Total node predictions generated (by outcome) |
| `predictive_scheduler_drain_candidates` | Gauge | Current count of predicted consolidation candidates |
| `predictive_scheduler_safe_nodes` | Gauge | Current count of safe waterline nodes |
| `predictive_scheduler_scheduling_decisions_total` | Counter | Placement decisions scored by the scheduler |
| `predictive_scheduler_evictions_total` | Counter | Gentle evictions executed |
| `predictive_scheduler_eviction_failures_total` | Counter | Evictions blocked by safety or PDBs |
| `predictive_scheduler_cpu_requested` | Gauge | Total CPU requested in millicores |
| `predictive_scheduler_cpu_used` | Gauge | Total CPU used in millicores |
| `predictive_scheduler_memory_requested` | Gauge | Total memory requested in bytes |
| `predictive_scheduler_memory_used` | Gauge | Total memory used in bytes |
| `predictive_scheduler_cluster_waste_ratio` | Gauge | Waste percentage (CPU, memory, composite) |
| `predictive_scheduler_estimated_cost` | Gauge | Estimated hourly infrastructure cost ($ USD) |
| `predictive_scheduler_estimated_savings` | Gauge | Estimated hourly infrastructure savings ($ USD) |
| `predictive_scheduler_prediction_accuracy` | Gauge | Empirical accuracy ratio (0.0 - 1.0) |

---

## Structured JSON Logging
All components output structured JSON logs to stdout:

```json
{
  "time": "2026-09-06T00:39:41.505Z",
  "level": "INFO",
  "component": "predictive-controller",
  "msg": "Simulation completed successfully",
  "safeNodes": 7,
  "drainCandidates": 1,
  "potentialHourlySavings": 0.096,
  "elapsedMs": 3
}
```

In AWS EKS, these logs are automatically captured by FluentBit / CloudWatch Container Insights and streamed to AWS CloudWatch Log Groups.

---

## Grafana Dashboards
Five production dashboard templates are located in `dashboards/grafana/`:
1. **Dashboard 1 — Cluster Overview**: Node count, pod count, CPU & memory utilization, requests vs usage.
2. **Dashboard 2 — Predictive Scheduling**: Real-time count of safe nodes, drain candidates, placement decisions, and prediction confidence.
3. **Dashboard 3 — Performance**: Webhook admission latency quantiles (P50, P95, P99) and request throughput.
4. **Dashboard 4 — FinOps & Infrastructure Waste**: Hourly cost, estimated hourly/monthly savings, and structural waste breakdown.
5. **Dashboard 5 — Experiment Comparison**: Control vs Treatment side-by-side waste ratio, node count delta, and measured percentage improvement.
