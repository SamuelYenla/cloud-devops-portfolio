output "vpc_id" {
  description = "Simulated data center VPC, consumed by the peering stack"
  value       = aws_vpc.dc.id
}

output "vpc_cidr" {
  description = "Data center CIDR, used for the return route in the peering stack"
  value       = aws_vpc.dc.cidr_block
}

output "route_table_id" {
  description = "Data center route table, which the peering stack adds the return route to"
  value       = aws_route_table.dc.id
}

output "host_private_ip" {
  description = "Source MySQL address. The migration dump targets this over the peering link."
  value       = aws_instance.dc.private_ip
}

output "host_public_dns" {
  description = "Wiki URL before migration, to confirm the source is serving"
  value       = "http://${aws_instance.dc.public_dns}:8000"
}

output "instance_id" {
  description = "For SSM Session Manager access"
  value       = aws_instance.dc.id
}

output "db_name" {
  value = var.db_name
}

output "db_user" {
  value = var.db_user
}
