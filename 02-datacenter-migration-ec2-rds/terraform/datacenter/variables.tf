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
  description = "CIDR for the simulated data center VPC. Must not overlap the target VPC - the original project put both on 10.0.0.0/16, which is why it could never be peered."
  type        = string
  default     = "10.10.0.0/16"
}

variable "target_vpc_cidr" {
  description = "CIDR of the target AWS VPC, allowed to reach MySQL here so the migration dump can cross the peering link"
  type        = string
  default     = "10.20.0.0/16"
}

variable "instance_type" {
  description = "Source host size. The sizing sheet specified 1 vCPU / 1 GB per host; t3.micro is the current-generation equivalent."
  type        = string
  default     = "t3.micro"
}

variable "db_name" {
  description = "Wiki database name"
  type        = string
  default     = "wiki"
}

variable "db_user" {
  description = "Wiki database user"
  type        = string
  default     = "wiki"
}

variable "db_password" {
  description = "MySQL password on the simulated data center host. This is a throwaway source environment that is destroyed after the migration, so it is a plain variable rather than a generated secret - the RDS credential in the data stack is generated and stored in SSM."
  type        = string
  default     = "wiki-dc-password"
  sensitive   = true
}

variable "seed_pages" {
  description = "Number of wiki pages to seed, so the migration has verifiable content"
  type        = number
  default     = 500
}
