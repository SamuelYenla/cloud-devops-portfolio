# Simulated corporate data center: the migration source.
#
# Deliberately a separate VPC with a non-overlapping CIDR. The original project
# put the on-premises hosts on 10.0.0.x and the AWS VPC on 10.0.0.0/16, which
# overlap - no VPN, Direct Connect or peering can join those, which is why its
# VPC console showed "Network connections (0)".

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*"]
  }
}

# ---------- Network ----------
resource "aws_vpc" "dc" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = { Name = "${var.name}-dc-vpc" }
}

resource "aws_internet_gateway" "dc" {
  vpc_id = aws_vpc.dc.id

  tags = { Name = "${var.name}-dc-igw" }
}

# A single subnet in one AZ. This environment is decommissioned at cutover, so
# spreading it across AZs would cost money to protect something being deleted.
resource "aws_subnet" "dc" {
  vpc_id                  = aws_vpc.dc.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 8, 0)
  availability_zone       = data.aws_availability_zones.available.names[0]
  map_public_ip_on_launch = true

  tags = { Name = "${var.name}-dc-public" }
}

resource "aws_route_table" "dc" {
  vpc_id = aws_vpc.dc.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.dc.id
  }

  tags = { Name = "${var.name}-dc-rt" }
}

resource "aws_route_table_association" "dc" {
  subnet_id      = aws_subnet.dc.id
  route_table_id = aws_route_table.dc.id
}

# ---------- Security ----------
resource "aws_security_group" "dc" {
  name        = "${var.name}-dc"
  description = "Simulated data center host"
  vpc_id      = aws_vpc.dc.id

  tags = { Name = "${var.name}-dc" }
}

# No ingress rule for SSH: access is via SSM Session Manager, so the host needs
# no open management port and no key pair.
resource "aws_vpc_security_group_ingress_rule" "dc_http" {
  security_group_id = aws_security_group.dc.id
  description       = "Wiki HTTP from within the data center"
  cidr_ipv4         = var.vpc_cidr
  from_port         = 8000
  to_port           = 8000
  ip_protocol       = "tcp"
}

# The migration dump crosses the peering link on this rule.
resource "aws_vpc_security_group_ingress_rule" "dc_mysql_from_target" {
  security_group_id = aws_security_group.dc.id
  description       = "MySQL from the target AWS VPC, for the migration dump"
  cidr_ipv4         = var.target_vpc_cidr
  from_port         = 3306
  to_port           = 3306
  ip_protocol       = "tcp"
}

resource "aws_vpc_security_group_egress_rule" "dc_all" {
  security_group_id = aws_security_group.dc.id
  description       = "Package installs during bootstrap"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
}

# ---------- Instance role ----------
data "aws_iam_policy_document" "assume_ec2" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dc" {
  name               = "${var.name}-dc-host"
  assume_role_policy = data.aws_iam_policy_document.assume_ec2.json
}

resource "aws_iam_role_policy_attachment" "dc_ssm" {
  role       = aws_iam_role.dc.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "dc" {
  name = "${var.name}-dc-host"
  role = aws_iam_role.dc.name
}

# ---------- Host ----------
resource "aws_instance" "dc" {
  ami                    = data.aws_ami.ubuntu.id
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.dc.id
  vpc_security_group_ids = [aws_security_group.dc.id]
  iam_instance_profile   = aws_iam_instance_profile.dc.name

  root_block_device {
    volume_size = 8
    volume_type = "gp3"
    encrypted   = true
  }

  metadata_options {
    http_tokens   = "required" # IMDSv2
    http_endpoint = "enabled"
  }

  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    db_name      = var.db_name
    db_user      = var.db_user
    db_password  = var.db_password
    seed_pages   = var.seed_pages
    app_py       = file("${path.module}/../../app/wiki/app.py")
    requirements = file("${path.module}/../../app/wiki/requirements.txt")
    schema_sql   = file("${path.module}/../../app/wiki/schema.sql")
  })

  # Re-bootstrap if the application or its bootstrap script changes.
  user_data_replace_on_change = true

  tags = { Name = "${var.name}-dc-host" }
}
