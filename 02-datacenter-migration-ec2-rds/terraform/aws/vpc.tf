# Target VPC and the peering link to the simulated data center.
#
# This stack deliberately holds the network, database and application tiers
# together. 01-eks-ordering-platform splits its stacks because the state
# bucket has prevent_destroy and a different lifecycle; here everything is
# created and destroyed in the same session, so splitting would only add
# apply/destroy cycles and remote-state plumbing.

data "aws_availability_zones" "available" {
  state = "available"
}

# The one genuine cross-stack dependency: the data center is applied first and
# torn down last, so it lives in its own state.
data "terraform_remote_state" "datacenter" {
  backend = "s3"

  config = {
    bucket = var.state_bucket
    key    = "02-datacenter-migration-ec2-rds/datacenter/terraform.tfstate"
    region = var.region
  }
}

locals {
  azs     = slice(data.aws_availability_zones.available.names, 0, 2)
  dc      = data.terraform_remote_state.datacenter.outputs
  dc_cidr = data.terraform_remote_state.datacenter.outputs.vpc_cidr
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"

  name = "${var.name}-vpc"
  cidr = var.vpc_cidr
  azs  = local.azs

  public_subnets   = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i)]      # ALB
  private_subnets  = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i + 10)] # app tier
  database_subnets = [for i in range(2) : cidrsubnet(var.vpc_cidr, 8, i + 20)] # RDS

  # The database tier is what the original project lacked. Its second private
  # subnet existed only to satisfy the RDS two-AZ subnet group requirement,
  # which is a constraint being met, not a high-availability design.
  create_database_subnet_group = true

  # A t4g.nano NAT instance would save ~$29/month, but only under 24/7
  # operation. Against the teardown-every-session rule the managed gateway
  # costs about $0.16 more per session, which does not justify hand-rolled
  # iptables masquerade as a dependency of every demo. See docs/cost.md.
  enable_nat_gateway = true
  single_nat_gateway = true

  enable_dns_hostnames = true # RDS endpoint resolution needs both
  enable_dns_support   = true
}

# ---------- Peering to the data center ----------
# The connection the original project never had. Its VPC console showed
# "Network connections (0)" because on-premises and AWS both sat on
# 10.0.0.0/16 - overlapping ranges cannot be joined by peering, VPN or
# Direct Connect.
resource "aws_vpc_peering_connection" "dc" {
  vpc_id      = module.vpc.vpc_id
  peer_vpc_id = local.dc.vpc_id
  auto_accept = true # same account and region

  tags = { Name = "${var.name}-to-dc" }
}

# Outbound: app and database subnets reach the data center.
resource "aws_route" "private_to_dc" {
  count = length(module.vpc.private_route_table_ids)

  route_table_id            = module.vpc.private_route_table_ids[count.index]
  destination_cidr_block    = local.dc_cidr
  vpc_peering_connection_id = aws_vpc_peering_connection.dc.id
}

# Return path, added to the data center's own route table.
resource "aws_route" "dc_to_aws" {
  route_table_id            = local.dc.route_table_id
  destination_cidr_block    = var.vpc_cidr
  vpc_peering_connection_id = aws_vpc_peering_connection.dc.id
}
