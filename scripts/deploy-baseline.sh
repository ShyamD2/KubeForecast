#!/usr/bin/env bash
set -euo pipefail

# Deploy predictive-scheduler Helm chart onto EKS
NAMESPACE="predictive-scheduler"
RELEASE_NAME="predictive-scheduler"

echo "=== Deploying Predictive Scheduler Helm Chart ==="

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install "${RELEASE_NAME}" ./charts/predictive-scheduler \
  --namespace "${NAMESPACE}" \
  -f ./charts/predictive-scheduler/values.yaml

echo "Verifying deployment rollout status:"
kubectl rollout status deployment/${RELEASE_NAME}-controller -n "${NAMESPACE}" --timeout=120s || true
kubectl rollout status deployment/${RELEASE_NAME}-webhook -n "${NAMESPACE}" --timeout=120s || true
kubectl rollout status deployment/${RELEASE_NAME}-scheduler -n "${NAMESPACE}" --timeout=120s || true
