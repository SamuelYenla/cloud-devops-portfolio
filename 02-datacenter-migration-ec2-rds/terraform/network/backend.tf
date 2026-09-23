terraform {
  backend "s3" {
    bucket       = "tfstate-188050967390-us-east-1"
    key          = "02-datacenter-migration-ec2-rds/network/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
