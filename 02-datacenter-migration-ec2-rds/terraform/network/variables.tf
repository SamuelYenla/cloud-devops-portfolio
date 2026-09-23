variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "name" {
  description = "Name prefix for all resources in this stack"
  type        = string
  default     = "dc-migration"
}

variable "vpc_cidr" {
  description = "CIDR for the target VPC. Must not overlap the simulated data center's 10.10.0.0/16."
  type        = string
  default     = "10.20.0.0/16"
}

variable "nat_instance_type" {
  description = "NAT instance size. t4g.nano at $0.0042/hr replaces a NAT gateway at $0.045/hr - about $29/month. See docs/cost.md for what that trades away."
  type        = string
  default     = "t4g.nano"
}
