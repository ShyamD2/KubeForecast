output "eks_cluster_name" {
  value       = module.eks.cluster_id
  description = "EKS Cluster Name"
}

output "eks_cluster_endpoint" {
  value       = module.eks.cluster_endpoint
  description = "EKS Cluster API Endpoint"
}

output "s3_snapshot_bucket" {
  value       = module.s3.bucket_id
  description = "S3 bucket for cluster snapshots"
}

output "irsa_role_arn" {
  value       = module.iam.role_arn
  description = "IAM Role ARN for Kubernetes ServiceAccount IRSA"
}

output "ecr_repositories" {
  value       = module.ecr.repository_urls
  description = "ECR repository URLs for Docker images"
}
