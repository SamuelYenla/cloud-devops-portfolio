# 01 - EKS Ordering Platform

Microservices ordering backend (menu, orders, payments-mock) on AWS EKS, modeled on a QSR digital-ordering platform.

## Layout
- `terraform/` - VPC, EKS, ECR, RDS, IAM
- `app/` - service source + Dockerfiles
- `k8s/` - Helm charts / manifests
- `.github/workflows/` - CI/CD pipelines
- `docs/` - architecture, cost notes, runbooks
