output "cluster_id" {
  value       = aws_eks_cluster.main.id
  description = "EKS cluster ID"
}

output "cluster_endpoint" {
  value       = aws_eks_cluster.main.endpoint
  description = "EKS cluster API endpoint"
}

output "cluster_certificate_authority_data" {
  value       = aws_eks_cluster.main.certificate_authority[0].data
  description = "EKS cluster CA data"
}

output "oidc_provider_arn" {
  value       = aws_iam_openid_connect_provider.oidc.arn
  description = "ARN of the OIDC provider for IRSA"
}

output "oidc_provider_url" {
  value       = aws_iam_openid_connect_provider.oidc.url
  description = "URL of the OIDC provider for IRSA"
}
