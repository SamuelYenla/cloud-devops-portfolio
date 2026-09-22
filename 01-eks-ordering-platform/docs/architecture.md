# Architecture decisions

Why the pieces are shaped the way they are. Where a choice was made against the obvious
default, the reasoning is recorded rather than the choice alone.

## Terraform is split into independent stacks

`bootstrap`, `network`, `eks` and `ecr` each hold their own state under a separate S3 key,
rather than living in one root module.

The driver is the teardown rule. This project is destroyed at the end of every working session,
and separate stacks mean the expensive parts can be destroyed while cheap or slow-to-recreate
parts survive. `bootstrap` in particular must never be destroyed — it holds the bucket that
stores every other stack's state, and its bucket carries `prevent_destroy`.

The cost is that cross-stack references go through `terraform_remote_state` rather than direct
resource references, so Terraform cannot see the whole graph at once. In exchange, a broken EKS
stack can be destroyed and rebuilt without touching a working VPC — which happened twice while
building this.

## One NAT gateway, not one per AZ

A NAT gateway is roughly $0.045/hour plus data processing, and the textbook layout puts one in
each availability zone so that an AZ failure cannot strand the nodes in another.

This project runs one, shared across both AZs. At this scale the NAT gateway is the single
largest line on the bill, and AZ-level redundancy protects an uptime guarantee that a portfolio
project does not make. Production would use one per AZ.

## Nodes are spot instances

`t3.medium` on SPOT capacity. Interruption would delete a node with two minutes' notice, which
in production would need a handler to drain gracefully.

Here the workload is stateless and replicated across two nodes, so an interruption costs a few
seconds of reduced capacity. The saving is roughly 70% against on-demand. Across a full session
of building, that is the difference between noticing the bill and not.

## The VPC CNI addon installs before the node group

This one was learned the hard way, and cost about two hours.

`terraform-aws-modules/eks` sets `bootstrap_self_managed_addons = false` internally, so EKS does
not install the default networking addons for you. Addons declared in the module's `addons` map
default to `before_compute = false`, which places them in a resource that carries
`depends_on = [module.eks_managed_node_group]`.

That is a deadlock. The CNI waits for the node group, the node group waits for its nodes to
report `Ready`, and a node cannot report `Ready` without a CNI. After about thirty minutes EKS
gives up with `NodeCreationFailure: Unhealthy nodes in the kubernetes cluster` — a message about
nodes, for a problem that is not about nodes.

`vpc-cni` and `eks-pod-identity-agent` therefore set `before_compute = true`. `coredns` and
`kube-proxy` do not need it: a node reaches `Ready` on CNI alone.

## Cluster creator admin permissions are explicit

`enable_cluster_creator_admin_permissions = true` is set deliberately. It defaults to `false` in
module v21, having defaulted to `true` in v20.

Without it no access entry is created for the identity that ran Terraform, and
aws-iam-authenticator answers an unmapped identity with "not authenticated" rather than
"authenticated but forbidden" — so `kubectl` returns `401` with an empty `"user":{}` rather than
a `403`. That failure mode reads convincingly like a broken control plane, and was misdiagnosed
as exactly that before the real cause was found.

## Container images are distroless, non-root, and pinned by UID

The `menu` image is `gcr.io/distroless/static-debian12:nonroot` with a statically linked Go
binary — about 8MB, and zero OS packages means zero OS CVEs for Trivy to find.

`USER` is set to the numeric `65532` rather than the name `nonroot`. The kubelet enforces
`runAsNonRoot` before starting a container and must prove the user is not UID 0. It only has the
image config to work from, cannot resolve a username without reading `/etc/passwd` inside the
image, and so refuses to start a container whose user it cannot verify. Naming the UID
numerically in both the image and the pod spec is what makes the two agree.

## ECR tags are immutable

Originally mutable, so that a manual `latest` push during development would not be rejected.
Trivy flagged it (`AWS-0031`), and by then CI was publishing one commit-SHA tag per build with
manifests referencing that SHA — so nothing needed to overwrite a tag any more.

Immutability makes a deployed tag a permanent record of exactly what shipped. The cost is that
`:latest` can no longer be re-pointed, which is the intended effect rather than a side effect.

## The service drains before it exits

On `SIGTERM` the process flips readiness to `503`, keeps serving for five seconds, and only then
begins shutdown with a ten-second timeout for in-flight requests.

Kubernetes removes a pod from Service endpoints asynchronously. A process that exits immediately
on `SIGTERM` will refuse connections that `kube-proxy` is still routing to it, which surfaces as
a handful of failed requests on every rolling update. The five-second pause covers that window.
`terminationGracePeriodSeconds: 30` is set above the 5 + 10 the process needs.

## No CPU limit, memory limit only

Requests are set for both CPU and memory; only memory carries a limit.

A CPU limit is enforced by CFS throttling, which adds latency to a service that is already well
under its request. The request alone guarantees a floor at scheduling time. Memory is different
— it is incompressible, and a limit is what stops one pod taking down a node.
