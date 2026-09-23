output "vpc_id" {
  value = module.vpc.vpc_id
}

output "vpc_cidr" {
  value = module.vpc.vpc_cidr_block
}

output "public_subnet_ids" {
  description = "ALB subnets"
  value       = module.vpc.public_subnets
}

output "private_subnet_ids" {
  description = "Application tier subnets - no public IPs, egress via the NAT instance"
  value       = module.vpc.private_subnets
}

output "database_subnet_ids" {
  value = module.vpc.database_subnets
}

output "database_subnet_group_name" {
  description = "Consumed by the data stack instead of hand-rolling a subnet group"
  value       = module.vpc.database_subnet_group_name
}

output "private_route_table_ids" {
  description = "Consumed by the peering stack to route toward the data center"
  value       = module.vpc.private_route_table_ids
}

output "public_route_table_ids" {
  value = module.vpc.public_route_table_ids
}

output "nat_instance_id" {
  description = "For SSM access when debugging egress"
  value       = aws_instance.nat.id
}

output "nat_private_ip" {
  value = aws_instance.nat.private_ip
}
