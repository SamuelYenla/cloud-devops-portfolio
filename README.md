# Cloud DevOps Portfolio

[![CI](https://github.com/SamuelYenla/cloud-devops-portfolio/actions/workflows/ci.yml/badge.svg)](https://github.com/SamuelYenla/cloud-devops-portfolio/actions/workflows/ci.yml)

Hands-on AWS cloud and DevOps projects, built from scratch rather than from a template. Every
piece here was provisioned, broken, debugged and rebuilt — the writeups in each project cover
the failures as well as the finished state.

| # | Project | Stack | Status |
|---|---------|-------|--------|
| 01 | [EKS Ordering Platform](01-eks-ordering-platform/) | Terraform, EKS, ECR, Go, GitHub Actions, Trivy | Building |

## How to read this repo

Each project directory holds its own README with the architecture, how to stand it up, and what
it costs to run. Start with [01-eks-ordering-platform](01-eks-ordering-platform/).

Design decisions and the reasoning behind them live in each project's `docs/`. Where a decision
looks unusual, there is normally a paragraph explaining what was tried first and why it failed.

## Conventions

- **Terraform is split into independent stacks** with separate state, rather than one root module.
  A stack can be destroyed without disturbing the others.
- **Remote state in S3** with native locking, bootstrapped by a stack that is itself never
  destroyed.
- **Everything is torn down at the end of each session** (`terraform destroy`). These projects
  demonstrate infrastructure, not uptime, and an idle EKS cluster is about $2.50/day.
- **CI runs on every push**: build, test, lint, and security scanning across Go, Terraform,
  Kubernetes manifests and the built container images.

## Status and honesty

This is an active build, and the table above reflects what actually exists today rather than
what is planned. Anything listed in a project README as "not built yet" genuinely is not built
yet — nothing here is aspirational.
