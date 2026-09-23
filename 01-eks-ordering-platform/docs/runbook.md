# Runbook

Operational procedures, and the failures actually hit while building this. Every entry below
happened; none are hypothetical.

## Routine checks

```bash
aws eks update-kubeconfig --name ordering-platform-cluster --region us-east-1

kubectl get nodes                                  # both should be Ready
kubectl get pods -n ordering -o wide               # one pod per node
kubectl logs -n ordering -l app.kubernetes.io/name=menu --tail=20
kubectl rollout status deployment/menu -n ordering
```

Probe endpoints are excluded from the service's request logging, so a quiet log is normal — it
means no real traffic, not a dead process.

## Deploying a new image

CI publishes an image per service on every merge to `main`, so normally there is nothing to
build — just point the deployment at the tag the pipeline produced. The run summary prints it.

```bash
SERVICE=orders                       # or menu, payments-mock
REGISTRY="$(cd terraform/ecr && terraform output -raw registry)"
SHA="$(git rev-parse --short HEAD)"

kubectl set image -n ordering "deployment/${SERVICE}" \
  "${SERVICE}=${REGISTRY}/ordering-platform/${SERVICE}:${SHA}"
kubectl rollout status "deployment/${SERVICE}" -n ordering
```

To build from a working tree that has not been pushed:

```bash
cd app
docker buildx build --platform linux/amd64 \
  --build-arg "SERVICE=${SERVICE}" \
  -t "${REGISTRY}/ordering-platform/${SERVICE}:${SHA}" --push .
```

Update the tag in `k8s/<service>/deployment.yaml` to match, or the next `kubectl apply` rolls it
back to whatever is committed.

Rollback is `kubectl rollout undo deployment/<service> -n ordering`.

`orders` depends on `menu` and `payments-mock`. Deploy those first if several are changing, so
`orders` is never calling an endpoint that is mid-rollout.

---

## Failure: node group fails with `NodeCreationFailure`

**Symptom.** `terraform apply` sits on the node group for about 30 minutes, then fails with
`NodeCreationFailure: Unhealthy nodes in the kubernetes cluster`. Nodes exist in EC2 and boot
cleanly, but never join.

**Cause.** No CNI. The module sets `bootstrap_self_managed_addons = false`, so EKS installs no
default networking addons, and an addon without `before_compute = true` depends on the node
group — which is waiting for the nodes it is blocking.

**Check.** Look at what Terraform actually created, not at the nodes:

```bash
terraform state list | grep aws_eks_addon
```

If only `data.aws_eks_addon_version` entries appear and no `aws_eks_addon` resources, the addons
never ran. That is this bug.

**Fix.** `before_compute = true` on `vpc-cni` in `terraform/eks/main.tf`. Already applied here —
this entry exists so the symptom is recognisable if it recurs on a new stack.

**Cleanup.** A failed node group does *not* disappear on its own, and its EC2 instances keep
billing. The node group name carries a random suffix, so `describe-nodegroup --nodegroup-name
default` returns `ResourceNotFound` and looks like it is already gone. List first:

```bash
aws eks list-nodegroups --cluster-name ordering-platform-cluster
aws eks delete-nodegroup --cluster-name ordering-platform-cluster --nodegroup-name <full-name>
```

Deleting the node group terminates its instances. Deleting the cluster while a node group still
exists fails with `ResourceInUseException: Cluster has nodegroups attached`.

---

## Failure: pods stuck in `CreateContainerConfigError`

**Symptom.** Image pulls successfully — the events confirm it, with a size and a duration — and
then the container never starts. No application logs, because no process ever ran.

**Cause.** `runAsNonRoot: true` with an image whose `USER` is a name rather than a UID. The
kubelet cannot verify a non-numeric user is not root, so it refuses to start the container:

```
container has runAsNonRoot and image has non-numeric user (nonroot),
cannot verify user is non-root
```

**Fix.** Numeric UID in both places — `USER 65532:65532` in the Dockerfile, `runAsUser: 65532`
in the pod spec. Distroless's `nonroot` user is UID 65532.

**Generalisation.** A `CreateContainerConfigError` sits between a successful pull and a running
process. It rules out the registry, credentials, image architecture and networking for free.

---

## Failure: `kubectl` returns 401 with an empty user

**Symptom.** Every request returns `401 Unauthorized`. The audit log shows `"user":{}` — no
identity at all, not a rejected one — and the authenticator logs show no trace of the attempt.

**Cause.** Almost always a missing access entry, not a broken cluster.
`enable_cluster_creator_admin_permissions` defaults to `false` in module v21. An identity with
no access entry is *unauthenticated* rather than unauthorised, which is why it is a 401 and not
a 403, and why it reads like a platform failure.

**Check.**

```bash
aws eks list-access-entries --cluster-name ordering-platform-cluster
aws sts get-caller-identity     # confirm which identity you are actually using
```

Note that an AWS CLI older than roughly v2.13 does not know about access entries at all and will
silently omit `accessConfig` from `describe-cluster` output, which makes a correctly configured
cluster look misconfigured.

**Fix.** `enable_cluster_creator_admin_permissions = true`, already set in `terraform/eks`.

---

## Failure: image runs locally, crash-loops on the cluster

**Symptom.** `exec format error` in the pod logs.

**Cause.** Architecture mismatch. The nodes are amd64; an image built natively on Apple Silicon
is arm64.

**Fix.** Always `docker buildx build --platform linux/amd64`. The Dockerfile cross-compiles from
`$BUILDPLATFORM`, so this costs a few seconds rather than running the whole build under QEMU.

**Check what was actually pushed:**

```bash
aws ecr batch-get-image --repository-name ordering-platform/menu \
  --image-ids imageTag=<tag> --region us-east-1 --query 'images[0].imageManifest'
```

---

## Terraform state is locked

A killed `terraform apply` can leave a lock object behind.

```bash
aws s3 ls s3://tfstate-<account-id>-us-east-1/01-eks-ordering-platform/<stack>/
```

A `.tflock` alongside `terraform.tfstate` with no Terraform process running is stale. Confirm
nothing is actually running first (`ps aux | grep terraform`), then `terraform force-unlock <id>`
using the ID from the error message. Do not delete the lock object by hand.

## A saved plan is rejected as stale

`Saved plan is stale` means state changed after the plan was written — commonly because
something was removed with `terraform state rm`, or another apply ran. Re-plan; never try to
force a stale plan through.
