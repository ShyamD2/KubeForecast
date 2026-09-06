variable "cluster_name" {
  type        = string
  description = "EKS cluster name"
}

variable "kubernetes_version" {
  type        = string
  default     = "1.31"
  description = "Kubernetes version"
}

variable "subnet_ids" {
  type        = list(string)
  description = "Subnet IDs for EKS cluster and nodes"
}

variable "instance_types" {
  type        = list(string)
  default     = ["c7i-flex.large"]
  description = "EC2 instance types for node group"
}

variable "desired_capacity" {
  type        = number
  default     = 4
  description = "Desired number of worker nodes"
}

variable "min_capacity" {
  type        = number
  default     = 2
  description = "Minimum number of worker nodes"
}

variable "max_capacity" {
  type        = number
  default     = 10
  description = "Maximum number of worker nodes"
}

variable "tags" {
  type        = map(string)
  default     = {}
  description = "Resource tags"
}
