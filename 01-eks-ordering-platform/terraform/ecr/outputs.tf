output "repository_urls" {
  description = "Service name to repository URL, used by CI when pushing images"
  value       = { for k, v in aws_ecr_repository.this : k => v.repository_url }
}

output "registry" {
  description = "Registry hostname, used for docker login"
  value       = split("/", values(aws_ecr_repository.this)[0].repository_url)[0]
}
