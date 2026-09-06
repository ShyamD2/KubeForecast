#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="${1:-./experiments/results}"
mkdir -p "${OUTPUT_DIR}"

echo "=== Collecting Prometheus Metrics from Controller ==="

# Forward metrics port if running locally
kubectl port-forward svc/predictive-scheduler-controller 8080:8080 -n predictive-scheduler &
PID=$!
sleep 3

curl -s http://localhost:8080/metrics > "${OUTPUT_DIR}/raw_metrics.prom" || true
kill $PID || true

echo "Metrics snapshot saved to ${OUTPUT_DIR}/raw_metrics.prom"
