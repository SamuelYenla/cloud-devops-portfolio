output "role_arn" {
  description = "Set as AWS_ROLE_ARN in the workflow; no secrets required"
  value       = aws_iam_role.ci.arn
}

output "oidc_provider_arn" {
  value = aws_iam_openid_connect_provider.github.arn
}

output "trusted_subjects" {
  description = "The OIDC subjects permitted to assume the role"
  value       = local.trusted_subjects
}
