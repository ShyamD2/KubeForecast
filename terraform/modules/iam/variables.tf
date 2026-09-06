variable "environment" {
  type        = string
  description = "Environment name"
}

variable "oidc_provider_arn" {
  type        = string
  description = "EKS OIDC provider ARN"
}

variable "oidc_provider_url" {
  type        = string
  description = "EKS OIDC provider URL"
}

variable "namespace" {
  type        = string
  default     = "predictive-scheduler"
  description = "Kubernetes namespace for the service account"
}

variable "service_account_name" {
  type        = string
  default     = "predictive-scheduler-sa"
  description = "Name of the Kubernetes service account"
}

variable "s3_bucket_arn" {
  type        = string
  description = "ARN of the S3 snapshot bucket"
}

variable "tags" {
  type        = map(string)
  default     = {}
  description = "Resource tags"
}
