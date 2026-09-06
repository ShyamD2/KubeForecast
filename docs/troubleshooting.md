# Troubleshooting & Operational Runbook

## 1. Workload Pods Stuck in `Pending`
### Symptoms
Pods with `spec.schedulerName: predictive-scheduler` remain in `Pending` state.

### Root Causes & Remedies
1. **Scheduler binary not running**:
   - Check scheduler pods: `kubectl get pods -n predictive-scheduler -l app.kubernetes.io/name=predictive-scheduler-scheduler`
   - Review logs: `kubectl logs -n predictive-scheduler -l app.kubernetes.io/name=predictive-scheduler-scheduler`
2. **Resource starvation on all candidate nodes**:
   - The scheduler plugin still enforces native resource filters. If no node has enough CPU/Memory to satisfy the pod's `requests`, it will stay pending.
   - Run `kubectl describe pod <pod-name>` to view standard scheduling failure events.

---

## 2. Pods Not Being Evicted from Drain Candidates
### Symptoms
Nodes identified as drain candidates continue running workloads indefinitely.

### Root Causes & Remedies
1. **Eviction is disabled by default**:
   - Verify `controller.eviction.enabled` in Helm values. By default, `eviction.enabled: false` (dry-run mode).
   - Check controller logs for `[DRY-RUN]` entries:
     ```bash
     kubectl logs -n predictive-scheduler -l app.kubernetes.io/name=predictive-scheduler-controller | grep DRY-RUN
     ```
2. **PodDisruptionBudget restrictions**:
   - If `kubectl get pdb -A` shows `ALLOWED DISRUPTIONS: 0`, the safety validator will refuse to evict the pod.
3. **Headroom threshold violation**:
   - The controller enforces `minAvailableCapacityPct: 0.20`. If safe destination nodes do not maintain at least 20% free capacity after absorbing the pod, the eviction is suppressed.

---

## 3. Webhook Admission Latency or Timeouts
### Symptoms
`kubectl apply` reports webhook timeout or connection refused on `/mutate`.

### Remedies
1. The webhook is configured with `failurePolicy: Ignore`, ensuring that pod creation succeeds even if the webhook is unresponsive.
2. Check webhook certificates: Verify that the secret `predictive-scheduler-webhook-tls` contains valid, unexpired TLS certificates.
3. Check pod network connectivity: Verify that security groups and network policies allow the Kubernetes API Server to connect to port 8443 on webhook pods.

---

## 4. S3 Snapshot Persistence Denied
### Symptoms
Controller logs error: `failed to persist cluster snapshot: AccessDenied`.

### Remedies
1. Verify the pod's projected token: `kubectl exec -it <controller-pod> -n predictive-scheduler -- cat /var/run/secrets/eks.amazonaws.com/serviceaccount/token`
2. Inspect IRSA trust relationship: Ensure the EKS OIDC provider URL matches the trust policy in the IAM role.
3. Verify S3 bucket name in Helm values matches the Terraform output.
