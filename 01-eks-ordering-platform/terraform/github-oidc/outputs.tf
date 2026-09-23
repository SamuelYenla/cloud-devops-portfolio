output "role_arn" {
  description = "Set as AWS_ROLE_ARN in the workflow; no secrets required"
  value       = aws_iam_role.ci.arn
}

output "oidc_provider_arn" {
  value = aws_iam_openid_connect_provider.github.arn
}

output "trusted_subject" {
  description = "The only OIDC subject permitted to assume the role"
  value       = "repo:${var.github_repository}:ref:refs/heads/${var.github_branch}"
}
