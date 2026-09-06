variable "environment" {
  type        = string
  description = "Environment name"
}

variable "force_destroy" {
  type        = bool
  default     = false
  description = "Allow bucket deletion even if non-empty (useful for ephemeral experiment teardown)"
}

variable "tags" {
  type        = map(string)
  default     = {}
  description = "Resource tags"
}
