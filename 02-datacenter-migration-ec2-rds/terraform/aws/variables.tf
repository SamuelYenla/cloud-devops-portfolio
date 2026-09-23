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

variable "state_bucket" {
  description = "Bucket holding the datacenter stack's state, read for the peering connection"
  type        = string
  default     = "tfstate-188050967390-us-east-1"
}

# ---------- Application tier ----------
variable "app_instance_type" {
  description = "Application tier size. The sizing sheet specified 1 vCPU / 1 GB."
  type        = string
  default     = "t4g.micro"
}

variable "desired_capacity" {
  description = "Running app instances. Default 1 keeps cost down; set to 2 to demonstrate the ALB balancing across AZs, screenshot it, then set it back. The ASG proves the HA claim whether or not two instances are running."
  type        = number
  default     = 1
}

# ---------- Database ----------
variable "db_instance_class" {
  description = "RDS size"
  type        = string
  default     = "db.t4g.micro"
}

variable "multi_az" {
  description = "Multi-AZ RDS doubles the database bill for a property no demo exercises. Set true for one apply to capture evidence of the standby, then set it back to false."
  type        = bool
  default     = false
}

variable "db_name" {
  description = "Wiki database name. Must match the datacenter stack."
  type        = string
  default     = "wiki"
}

variable "db_user" {
  description = "Wiki database user. Must match the datacenter stack."
  type        = string
  default     = "wiki"
}
