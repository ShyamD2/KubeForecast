# FinOps Cost Engine & Multi-Dimensional Waste Model

## The Danger of CPU-Only Waste Measurement
A common mistake in Kubernetes cost optimization is measuring waste solely based on CPU utilization. In real-world enterprise environments, this is profoundly misleading:

1. **Memory-Bound Workloads**: Services such as Redis, Kafka, JVM microservices, Elasticsearch, and in-memory caches consume substantial memory while utilizing very little CPU. A node hosting such workloads may show 15% CPU load but 90% memory allocation.
2. **Eviction Cascades**: If an optimization engine flags a node for consolidation based only on low CPU, attempting to migrate those pods will either fail or trigger out-of-memory (OOM) kills on destination nodes.
3. **Multi-Vector Bin-Packing**: Kubernetes nodes are multi-dimensional containers. Infrastructure cost is incurred on the entire instance (CPU + Memory + Network + Storage), not CPU alone.

---

## Waste Calculation Formulas

### 1. Resource Waste
$$\text{CPU Waste \%} = \frac{\sum R_{\text{cpu}} - \sum U_{\text{cpu}}}{\sum R_{\text{cpu}}} \times 100$$

$$\text{Memory Waste \%} = \frac{\sum R_{\text{mem}} - \sum U_{\text{mem}}}{\sum R_{\text{mem}}} \times 100$$

### 2. Structural Cluster Waste
Structural waste measures the combined capacity provisioned at the cloud VM level that is never utilized by any active workload:
$$\text{Structural Waste \%} = \left(\frac{(A_{\text{cpu}} - U_{\text{cpu}})/A_{\text{cpu}} + (A_{\text{mem}} - U_{\text{mem}})/A_{\text{mem}}}{2}\right) \times 100$$

---

## Configurable Pricing Model
Instance pricing is loaded dynamically from configuration rather than hardcoded:

```yaml
pricing:
  m5.large: 0.096    # $0.096 / hr
  m5.xlarge: 0.192   # $0.192 / hr
  c5.large: 0.085    # $0.085 / hr
  r5.large: 0.126    # $0.126 / hr
  t3.large: 0.0832   # $0.0832 / hr
```

### Financial Savings Delta
$$\text{Hourly Savings} = \text{Cost}_{\text{Control}} - \text{Cost}_{\text{Treatment}}$$
$$\text{Savings \%} = \frac{\text{Hourly Savings}}{\text{Cost}_{\text{Control}}} \times 100$$
