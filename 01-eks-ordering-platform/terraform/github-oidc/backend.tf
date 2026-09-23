terraform {
  backend "s3" {
    bucket       = "tfstate-188050967390-us-east-1"
    key          = "01-eks-ordering-platform/github-oidc/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
