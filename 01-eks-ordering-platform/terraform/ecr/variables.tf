variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Name prefix for repositories, shared with the other stacks"
  type        = string
  default     = "ordering-platform"
}

variable "services" {
  description = "Services that get a repository"
  type        = set(string)
  default     = ["menu", "orders", "payments-mock"]
}

variable "tagged_image_count" {
  description = "How many tagged images to keep before the oldest are expired"
  type        = number
  default     = 10
}
