output "repository_urls" {
  value       = { for k, v in aws_ecr_repository.repos : k => v.repository_url }
  description = "Map of repository names to their registry URLs"
}
