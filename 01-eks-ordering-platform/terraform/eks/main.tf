data "terraform_remote_state" "network" {
  backend = "s3"

  config = {
    bucket = "tfstate-188050967390-us-east-1"
    key    = "01-eks-ordering-platform/network/terraform.tfstate"
    region = "us-east-1"
  }
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 21.0"

  name               = "${var.name}-cluster"
  kubernetes_version = var.cluster_version

  # Public+private endpoint: no bastion/VPN in this portfolio setup, kubectl
  # needs to reach the API from a local machine.
  endpoint_public_access = true

  vpc_id     = data.terraform_remote_state.network.outputs.vpc_id
  subnet_ids = data.terraform_remote_state.network.outputs.private_subnet_ids

  enable_irsa = true

  # Grants the Terraform caller an access entry; without it kubectl gets a bare 401.
  enable_cluster_creator_admin_permissions = true

  # The module sets bootstrap_self_managed_addons = false, so nothing installs a CNI
  # on its own. vpc-cni must land before the node group or nodes never reach Ready
  # and the node group fails with NodeCreationFailure.
  addons = {
    coredns    = { most_recent = true }
    kube-proxy = { most_recent = true }

    vpc-cni = {
      most_recent    = true
      before_compute = true
    }
    eks-pod-identity-agent = {
      most_recent    = true
      before_compute = true
    }
  }

  eks_managed_node_groups = {
    default = {
      ami_type       = "AL2023_x86_64_STANDARD"
      instance_types = [var.node_instance_type]
      capacity_type  = var.node_capacity_type

      min_size     = var.node_min_size
      max_size     = var.node_max_size
      desired_size = var.node_desired_size
    }
  }
}
