resource "aws_ecr_repository" "this" {
  for_each = var.services

  name = "${var.name}/${each.value}"

  # Tags stay mutable so a manual `latest` push during development isn't rejected.
  # CI pushes commit-SHA tags, which are effectively immutable anyway.
  image_tag_mutability = "MUTABLE"

  # This repo is torn down at the end of each session; without this, destroy
  # fails on any repository that still holds images.
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }
}

resource "aws_ecr_lifecycle_policy" "this" {
  for_each = aws_ecr_repository.this

  repository = each.value.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after 1 day"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 1
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Keep only the most recent ${var.tagged_image_count} tagged images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = var.tagged_image_count
        }
        action = { type = "expire" }
      },
    ]
  })
}
