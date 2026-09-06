#!/usr/bin/env bash
set -euo pipefail

# Run the complete deterministic predictive scheduling benchmark suite
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== Running Kubernetes Predictive Scheduling Benchmark Suite ==="
cd "${ROOT_DIR}"

mkdir -p ./experiments/results

go run ./cmd/simulator/main.go --scenario all --output-dir ./experiments/results

echo ""
echo "Benchmark completed successfully."
echo "Results exported to:"
echo "  - JSON: ./experiments/results/benchmark_summary.json"
echo "  - CSV:  ./experiments/results/benchmark_summary.csv"
