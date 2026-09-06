#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="experiment-treatment"
echo "=== Deploying Treatment Workloads (Predictive Scheduling Enabled) ==="

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${NAMESPACE}" predictive-scheduler/group=treatment --overwrite

for f in ./experiments/workloads/*.yaml; do
  kubectl apply -f "$f"
done

echo "Treatment workloads submitted."
kubectl get pods -n "${NAMESPACE}" -o wide
