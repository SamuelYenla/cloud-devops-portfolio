# Target VPC: three subnet tiers across two AZs.
#
# The database tier is what the original project lacked. It had one public and
# two private subnets, and the second private subnet existed only because RDS
# requires a subnet group spanning two AZs - an artifact of a constraint rather
# than a high-availability design.

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.available.names, 0, 2)
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

  create_database_subnet_group = true

  # No managed NAT gateway - a t4g.nano NAT instance below does the same job
  # for about $29/month less. 01-eks-ordering-platform makes the opposite call
  # because EKS needs the throughput and the managed failover.
  enable_nat_gateway = false

  enable_dns_hostnames = true # RDS endpoint resolution needs both
  enable_dns_support   = true
}

# ---------- NAT instance ----------
data "aws_ami" "al2023_arm" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-arm64"]
  }
}

resource "aws_security_group" "nat" {
  name        = "${var.name}-nat"
  description = "NAT instance for the private subnets"
  vpc_id      = module.vpc.vpc_id

  tags = { Name = "${var.name}-nat" }
}

resource "aws_vpc_security_group_ingress_rule" "nat_from_vpc" {
  security_group_id = aws_security_group.nat.id
  description       = "Traffic being routed out from the private subnets"
  cidr_ipv4         = var.vpc_cidr
  ip_protocol       = "-1"
}

resource "aws_vpc_security_group_egress_rule" "nat_all" {
  security_group_id = aws_security_group.nat.id
  description       = "Forwarded traffic to the internet"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

data "aws_iam_policy_document" "assume_ec2" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "nat" {
  name               = "${var.name}-nat"
  assume_role_policy = data.aws_iam_policy_document.assume_ec2.json
}

resource "aws_iam_role_policy_attachment" "nat_ssm" {
  role       = aws_iam_role.nat.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "nat" {
  name = "${var.name}-nat"
  role = aws_iam_role.nat.name
}

resource "aws_instance" "nat" {
  ami                    = data.aws_ami.al2023_arm.id
  instance_type          = var.nat_instance_type
  subnet_id              = module.vpc.public_subnets[0]
  vpc_security_group_ids = [aws_security_group.nat.id]
  iam_instance_profile   = aws_iam_instance_profile.nat.name

  # Without this the instance drops packets not addressed to it, which is
  # every packet it is supposed to forward.
  source_dest_check = false

  root_block_device {
    volume_size = 8
    volume_type = "gp3"
    encrypted   = true
  }

  metadata_options {
    http_tokens   = "required" # IMDSv2
    http_endpoint = "enabled"
  }

  user_data = templatefile("${path.module}/nat_user_data.sh.tftpl", {
    vpc_cidr = var.vpc_cidr
  })

  user_data_replace_on_change = true

  tags = { Name = "${var.name}-nat" }
}

# Default route for each private route table. The database subnets share these
# route tables, which is harmless - RDS never uses the route.
resource "aws_route" "private_nat" {
  count = length(module.vpc.private_route_table_ids)

  route_table_id         = module.vpc.private_route_table_ids[count.index]
  destination_cidr_block = "0.0.0.0/0"
  network_interface_id   = aws_instance.nat.primary_network_interface_id
}
