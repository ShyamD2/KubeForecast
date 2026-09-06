#!/usr/bin/env bash
set -euo pipefail

ENV="${1:-dev}"
REGION="${2:-us-east-1}"

echo "=== Tearing Down AWS Infrastructure for Environment: ${ENV} ==="

cd "$(dirname "${BASH_SOURCE[0]}")/../terraform/environments/${ENV}"

terraform destroy -auto-approve -var="aws_region=${REGION}"

echo "Environment ${ENV} destroyed cleanly."
