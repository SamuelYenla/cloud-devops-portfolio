variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Name prefix, shared with the other stacks"
  type        = string
  default     = "ordering-platform"
}

variable "github_repository" {
  description = "owner/repo allowed to assume the CI role"
  type        = string
  default     = "SamuelYenla/cloud-devops-portfolio"

  validation {
    condition     = can(regex("^[^/]+/[^/]+$", var.github_repository))
    error_message = "Must be in owner/repo form, with no leading or trailing slash."
  }
}

variable "github_branch" {
  description = "Branch allowed to assume the role. Only this ref can publish images."
  type        = string
  default     = "main"
}

variable "ecr_repository_prefix" {
  description = "ECR namespace the CI role may push to"
  type        = string
  default     = "ordering-platform"
}
