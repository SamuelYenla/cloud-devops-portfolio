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

# GitHub now issues subjects carrying immutable numeric IDs alongside the names,
# e.g. repo:owner@83687702/repo@1379351988:ref:refs/heads/main. Pinning the IDs
# is what stops trust transferring to whoever claims the login after a rename.
# Find them with:
#   gh api repos/OWNER/REPO --jq '{repo: .id, owner: .owner.id}'
variable "github_owner_id" {
  description = "Numeric GitHub account ID of the repository owner"
  type        = number
  default     = 83687702
}

variable "github_repository_id" {
  description = "Numeric GitHub repository ID"
  type        = number
  default     = 1379351988
}

variable "ecr_repository_prefix" {
  description = "ECR namespace the CI role may push to"
  type        = string
  default     = "ordering-platform"
}
