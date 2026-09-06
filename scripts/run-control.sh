#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="experiment-control"
echo "=== Deploying Control Baseline Workloads (Normal Kubernetes Scheduling) ==="

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${NAMESPACE}" predictive-scheduler/group=control --overwrite

# Apply workloads configured with control namespace
for f in ./experiments/workloads/*.yaml; do
  sed 's/namespace: experiment-treatment/namespace: experiment-control/g' "$f" | \
  sed 's/predictive-scheduler\/group: treatment/predictive-scheduler\/group: control/g' | \
  kubectl apply -f -
done

echo "Control workloads submitted."
kubectl get pods -n "${NAMESPACE}" -o wide
