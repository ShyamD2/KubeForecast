#!/usr/bin/env bash
set -euo pipefail

ENV="${1:-dev}"
REGION="${2:-us-east-1}"

echo "=== Provisioning AWS EKS Infrastructure for Environment: ${ENV} ==="

cd "$(dirname "${BASH_SOURCE[0]}")/../terraform/environments/${ENV}"

terraform init
terraform apply -auto-approve -var="aws_region=${REGION}"

CLUSTER_NAME="$(terraform output -raw eks_cluster_name)"
echo "Updating kubeconfig for cluster: ${CLUSTER_NAME} in region ${REGION}..."
aws eks update-kubeconfig --name "${CLUSTER_NAME}" --region "${REGION}"

echo "Cluster ready! Verifying nodes:"
kubectl get nodes -o wide
