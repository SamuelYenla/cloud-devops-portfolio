output "wiki_url" {
  description = "Public entry point. Serves the data center database until cutover, RDS after."
  value       = "http://${aws_lb.wiki.dns_name}"
}

output "vpc_id" {
  value = module.vpc.vpc_id
}

output "rds_endpoint" {
  description = "Migration target. scripts/migrate.sh restores the dump here."
  value       = aws_db_instance.wiki.address
}

output "rds_port" {
  value = aws_db_instance.wiki.port
}

output "db_host_parameter" {
  description = "The cutover switch: update this, restart the service, and the app serves from RDS"
  value       = aws_ssm_parameter.db_host.name
}

output "db_password_parameter" {
  value = aws_ssm_parameter.db_password.name
}

output "autoscaling_group_name" {
  value = aws_autoscaling_group.app.name
}

output "peering_connection_id" {
  description = "The link the original project never had"
  value       = aws_vpc_peering_connection.dc.id
}

output "dc_host_private_ip" {
  description = "Migration source, reachable from this VPC over peering"
  value       = local.dc.host_private_ip
}
