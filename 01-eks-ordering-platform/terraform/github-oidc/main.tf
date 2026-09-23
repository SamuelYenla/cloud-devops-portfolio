data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  oidc_host = "token.actions.githubusercontent.com"

  ecr_repositories = "arn:${data.aws_partition.current.partition}:ecr:${var.region}:${data.aws_caller_identity.current.account_id}:repository/${var.ecr_repository_prefix}/*"

  owner = split("/", var.github_repository)[0]
  repo  = split("/", var.github_repository)[1]

  # GitHub issues one of two subject formats depending on the account. The
  # second carries immutable numeric IDs, which is what this repository
  # actually receives — verified by decoding a live token, since the trust
  # policy fails silently with "Not authorized" when the format is wrong.
  # Both are listed so a change in GitHub's behaviour cannot break publishing.
  trusted_subjects = [
    "repo:${var.github_repository}:ref:refs/heads/${var.github_branch}",
    "repo:${local.owner}@${var.github_owner_id}/${local.repo}@${var.github_repository_id}:ref:refs/heads/${var.github_branch}",
  ]
}

# Read the current certificate rather than pinning a thumbprint by hand, so a
# certificate rotation at GitHub does not silently break authentication.
data "tls_certificate" "github" {
  url = "https://${local.oidc_host}"
}

resource "aws_iam_openid_connect_provider" "github" {
  url            = "https://${local.oidc_host}"
  client_id_list = ["sts.amazonaws.com"]

  # The last certificate in the chain is the root CA, which is what AWS expects.
  # certificates[0] is the leaf, and pinning that rotates every few months.
  thumbprint_list = [
    data.tls_certificate.github.certificates[length(data.tls_certificate.github.certificates) - 1].sha1_fingerprint
  ]
}

data "aws_iam_policy_document" "assume_role" {
  statement {
    effect  = "Allow"
    actions = ["sts:AssumeRoleWithWebIdentity"]

    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }

    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:aud"
      values   = ["sts.amazonaws.com"]
    }

    # The sub claim is the whole security boundary. Without it any repository on
    # GitHub could assume this role. Both accepted values name one repository
    # and one branch — no wildcards.
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_host}:sub"
      values   = local.trusted_subjects
    }
  }
}

resource "aws_iam_role" "ci" {
  name                 = "${var.name}-github-actions"
  description          = "Assumed by GitHub Actions via OIDC to publish images"
  assume_role_policy   = data.aws_iam_policy_document.assume_role.json
  max_session_duration = 3600
}

data "aws_iam_policy_document" "ecr_push" {
  # GetAuthorizationToken issues an account-wide token and cannot be scoped to
  # a repository, so it has to stand alone on "*".
  statement {
    sid       = "AuthenticateToRegistry"
    effect    = "Allow"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid    = "PushToOrderingPlatformRepos"
    effect = "Allow"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:BatchGetImage",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeImages",
      "ecr:GetDownloadUrlForLayer",
      "ecr:InitiateLayerUpload",
      "ecr:PutImage",
      "ecr:UploadLayerPart",
    ]
    resources = [local.ecr_repositories]
  }
}

resource "aws_iam_role_policy" "ecr_push" {
  name   = "ecr-push"
  role   = aws_iam_role.ci.id
  policy = data.aws_iam_policy_document.ecr_push.json
}
