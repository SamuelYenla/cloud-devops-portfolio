# 01 — EKS Ordering Platform

A microservices ordering backend on AWS EKS, modelled on a quick-service-restaurant digital
ordering platform. Built with Terraform, deployed from ECR, tested and scanned in CI.

## Architecture

```mermaid
flowchart TD
    GH["GitHub Actions<br/>build · test · scan"]
    TF["Terraform CLI"]

    subgraph Managed["AWS-managed account"]
        CP["EKS control plane<br/>Kubernetes 1.31"]
    end

    subgraph AWS["AWS account · us-east-1"]
        ECR["ECR<br/>immutable tags"]
        S3["S3<br/>Terraform state"]

        subgraph VPC["VPC 10.0.0.0/16"]
            IGW["Internet gateway"]

            subgraph Public["Public subnets · 2 AZs"]
                NAT["NAT gateway<br/>single, shared"]
            end

            subgraph Private["Private subnets · 2 AZs"]
                ENI["EKS ENIs<br/>private endpoint"]
                N1["Node · AZ-1<br/>t3.medium spot"]
                N2["Node · AZ-2<br/>t3.medium spot"]
            end
        end
    end

    GH -->|push| ECR
    TF -.->|state| S3

    N1 -->|API · in-VPC| ENI
    N2 -->|API · in-VPC| ENI
    ENI --- CP

    N1 -->|egress| NAT
    N2 -->|egress| NAT
    NAT --> IGW
    IGW -.->|image pull| ECR
```

Two `menu` pods run one per node, behind a ClusterIP Service. Three details in that diagram are
easy to draw wrong and worth stating plainly:

- **The control plane is not in this VPC.** It runs in an AWS-managed account. What sits in the
  private subnets is a pair of requester-managed ENIs — `describe-network-interfaces` shows them
  owned by this account but requested by an AWS-owned one. Node-to-API traffic reaches the
  control plane through those ENIs and never leaves the VPC.
- **Image pulls leave the VPC and come back.** There are no VPC endpoints here, so a node pulling
  from ECR egresses through the NAT gateway and the internet gateway to ECR's public endpoint.
  That path is metered per GB, which is part of why the image is kept at 8MB.
- **One NAT gateway, shared across both AZs.** One per AZ is the textbook layout; this halves the
  largest line on the bill in exchange for AZ-level redundancy a portfolio project does not need.

## What exists today

| Component | State |
|---|---|
| Terraform: state bucket + budget alarms | Built |
| Terraform: VPC, subnets, NAT, routing | Built |
| Terraform: EKS cluster, managed node group, core addons | Built |
| Terraform: ECR repositories with lifecycle rules | Built |
| `menu` service (Go) + manifests | Built, running |
| CI: build, test, lint, security scan | Built, green |
| CD: publish to ECR from CI via OIDC, no stored keys | Built |
| `orders`, `payments-mock` services | Not built yet |
| Automated deploy to the cluster | Not built yet — see below |
| RDS, ArgoCD, Prometheus/Grafana, ingress | Not built yet |

## Layout

```
terraform/
  bootstrap/     state bucket + budget alarms — created once, never destroyed
  network/       VPC, subnets, NAT gateway, routing
  eks/           cluster, managed node group, core addons
  ecr/           one repository per service
  github-oidc/   IAM role GitHub Actions assumes to publish images
app/menu/        Go service, Dockerfile, tests
k8s/             namespace + per-service manifests
docs/            architecture decisions, costs, runbook
```

Each Terraform directory is an independent stack with its own state key. `eks` reads the
network's outputs through `terraform_remote_state` rather than hardcoding subnet IDs.

## Standing it up

Requires Terraform ≥ 1.10, AWS credentials, `kubectl`, and Docker.

```bash
# 1. Once per account — creates the state bucket the other stacks write to
cd terraform/bootstrap && terraform init && terraform apply

# 2. Network, then the cluster (eks depends on network's outputs)
cd ../network && terraform init && terraform apply
cd ../eks     && terraform init && terraform apply    # ~15 min, mostly the control plane
cd ../ecr     && terraform init && terraform apply

# 3. Build and publish the service image
aws ecr get-login-password --region us-east-1 \
  | docker login --username AWS --password-stdin "$(cd terraform/ecr && terraform output -raw registry)"

cd app/menu
REPO="$(cd ../../terraform/ecr && terraform output -json repository_urls | jq -r .menu)"
docker buildx build --platform linux/amd64 -t "${REPO}:$(git rev-parse --short HEAD)" --push .

# 4. Deploy
aws eks update-kubeconfig --name ordering-platform-cluster --region us-east-1
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/menu/
kubectl rollout status deployment/menu -n ordering
```

The image **must** be built for `linux/amd64`. The nodes are amd64, so an image built natively
on an Apple Silicon machine will pull successfully and then crash-loop with `exec format error`.

## Trying it

The Service is ClusterIP — there is no ingress yet, deliberately, since a load balancer costs
more than the rest of the stack combined at this scale.

```bash
kubectl port-forward -n ordering service/menu 8080:80
```

```bash
curl localhost:8080/menu                    # full catalogue
curl 'localhost:8080/menu?category=drinks'  # filtered
curl localhost:8080/menu/burger-double      # one item
curl localhost:8080/healthz                 # liveness
curl localhost:8080/readyz                  # readiness
```

## Tearing down

Destroy in reverse dependency order. `bootstrap` is deliberately left alone — it holds the state
for everything else and its bucket is protected by `prevent_destroy`.

```bash
cd terraform/ecr     && terraform destroy   # force_delete drops images with the repos
cd ../eks            && terraform destroy   # delete node groups first if any failed to create
cd ../network        && terraform destroy
```

## How images get published

A push to `main` that passes every check ends with an image in ECR, tagged with the commit SHA.
There are no AWS access keys stored in GitHub: the workflow mints an OIDC token, and an IAM role
whose trust policy accepts exactly one repository on exactly one branch exchanges it for
temporary credentials. `id-token: write` is granted to the publishing job alone.

Deployment is still manual — `kubectl set image`, which the workflow prints in its run summary.
Automating it would mean CI holding credentials to a cluster that is destroyed at the end of
every session, so every run against a torn-down cluster would fail. That belongs with a GitOps
controller that reconciles when the cluster exists, rather than a pipeline step that assumes it.

## Further reading

- [docs/architecture.md](docs/architecture.md) — why the pieces are shaped this way
- [docs/costs.md](docs/costs.md) — what this costs per hour and where it goes
- [docs/runbook.md](docs/runbook.md) — operations, and the failures hit while building it
