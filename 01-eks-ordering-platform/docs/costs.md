# Costs

What this stack costs while it is running, and where the money actually goes. Figures are
us-east-1 list prices; spot is the live market rate at the time of writing and moves.

## Running total

| Component | Rate | Per hour |
|---|---|---|
| EKS control plane | $0.10/hr, flat | $0.100 |
| NAT gateway | $0.045/hr | $0.045 |
| 2 × `t3.medium` spot | ~$0.0165/hr each | $0.033 |
| EBS root volumes (module default, gp3) | $0.08/GB-month | ~$0.004 |
| **Total** | | **~$0.18/hr** |

That is roughly **$4.40/day**, or about **$131/month** if it were left running — which it is
not. The `bootstrap` stack sets a $50/month budget alarm with notifications well before that.

Spot is the clearest saving: `t3.medium` is $0.0416/hr on demand against ~$0.0165 on spot, so
the two nodes cost about 60% less than they otherwise would.

## The two things that dominate

**The control plane is fixed and unavoidable.** $0.10/hr is charged per cluster regardless of
whether anything runs on it. An empty cluster and a busy one cost the same. This is why the
teardown discipline matters more than any node-level optimisation — you cannot tune your way out
of $73/month, you can only stop paying it.

**The NAT gateway is the biggest thing you control.** At $0.045/hr it is a quarter of the bill,
and the standard one-per-AZ layout would make it half. Running a single shared NAT gateway is
the single largest saving in this stack. See [architecture.md](architecture.md) for what that
trades away.

## Metered extras

These are usage-based and small here, but they are the ones that surprise people:

- **NAT data processing** — $0.045/GB on top of the hourly rate. Image pulls go through it.
  Keeping images small (8MB, distroless) helps more than it looks like it should.
- **CloudWatch Logs ingestion** — $0.50/GB. The cluster has `api`, `audit` and `authenticator`
  logging enabled, and the audit log is chatty: control-plane components poll constantly, so it
  accrues even with zero application traffic. Worth disabling if a cluster is left up for days.
- **ECR storage** — $0.10/GB-month. The lifecycle rules cap tagged images at 10 and expire
  untagged ones after a day, so this stays negligible.
- **S3 state and Elastic IP** — effectively free. An EIP is only billed when it is *not*
  attached to anything.

## Teardown

The rule is that every session ends with `terraform destroy`. Order matters — see the teardown
section in the [project README](../README.md).

`bootstrap` is the exception and is never destroyed. It holds the state bucket for everything
else, its bucket carries `prevent_destroy`, and it costs essentially nothing: S3 storage for a
few hundred kilobytes of state, plus a budget alarm that is free.

What survives a teardown: the state bucket, the budget alarm, and the ECR repositories' *config*
in state — but not their images, since `force_delete = true` drops images along with the
repositories. A fresh session rebuilds and re-pushes.

## A note on failed builds

Two failed EKS cluster builds while debugging the CNI deadlock cost about 30 minutes each of
control plane plus node time — call it $0.20. The expensive part of a failure is not the
infrastructure, it is leaving something running while you work out what went wrong. Orphaned
resources are the real risk: a node group that fails to create can leave EC2 instances running
with nothing managing them, and nothing will clean those up for you.
