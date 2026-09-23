# Migration target: RDS MySQL 8.0.
#
# The source runs MySQL 5.7, which is past RDS standard support. Migrating
# 5.7 -> 8.0 is part of the point, so the version gap is deliberate.

resource "random_password" "db" {
  length  = 32
  special = false # avoids quoting problems in the bootstrap shell
}

# Parameter Store standard tier is free; Secrets Manager is $0.40/secret/month.
# It also deletes immediately, where Secrets Manager's 7-day soft delete would
# collide with the next apply under a teardown-every-session rule.
resource "aws_ssm_parameter" "db_password" {
  name        = "/${var.name}/db/password"
  description = "RDS credential for the wiki application"
  type        = "SecureString"
  value       = random_password.db.result
}

# The cutover switch. The application reads this at startup, so moving from
# the data center to RDS is a parameter change plus a service restart rather
# than a rebuild. scripts/migrate.sh writes it, which is why Terraform stops
# managing the value after creation.
resource "aws_ssm_parameter" "db_host" {
  name        = "/${var.name}/db/host"
  description = "Database the wiki points at. Starts at the data center, moves to RDS at cutover."
  type        = "String"
  value       = local.dc.host_private_ip

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_security_group" "db" {
  name        = "${var.name}-db"
  description = "RDS MySQL"
  vpc_id      = module.vpc.vpc_id

  tags = { Name = "${var.name}-db" }
}

# By security group reference, not CIDR: a CIDR rule would admit anything that
# happens to land in the subnet, including a future unrelated instance.
resource "aws_vpc_security_group_ingress_rule" "db_from_app" {
  security_group_id            = aws_security_group.db.id
  description                  = "MySQL from the application tier only"
  referenced_security_group_id = aws_security_group.app.id
  from_port                    = 3306
  to_port                      = 3306
  ip_protocol                  = "tcp"
}

resource "aws_db_instance" "wiki" {
  identifier     = "${var.name}-wiki"
  engine         = "mysql"
  engine_version = "8.0"
  instance_class = var.db_instance_class

  db_name  = var.db_name
  username = var.db_user
  password = random_password.db.result

  allocated_storage = 20 # RDS minimum
  storage_type      = "gp3"
  storage_encrypted = true

  db_subnet_group_name   = module.vpc.database_subnet_group_name
  vpc_security_group_ids = [aws_security_group.db.id]
  multi_az               = var.multi_az
  publicly_accessible    = false

  # Nothing here is worth retaining, and backups would add snapshot storage
  # cost to a stack that is destroyed after every session.
  backup_retention_period = 0
  skip_final_snapshot     = true
  deletion_protection     = false

  apply_immediately = true

  tags = { Name = "${var.name}-wiki" }
}
