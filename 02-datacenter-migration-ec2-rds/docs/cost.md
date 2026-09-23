# Cost

This is a portfolio artifact. It is built to be read, demoed briefly, and destroyed — so the
number that matters is **cost per session**, not cost per month.

## Running cost

`us-east-1`, defaults (`desired_capacity = 1`, `multi_az = false`):

| Item | $/hr | Share |
|---|---|---|
| ALB (+ ~1 LCU) | 0.030 | 33% |
| NAT gateway | 0.045 | 49% |
| RDS `db.t4g.micro`, single-AZ | 0.016 | 17% |
| DC host `t3.micro` | 0.0104 | 11% |
| App `t4g.micro` × 1 | 0.0084 | 9% |
| EBS ~24 GB gp3 | 0.0027 | 3% |
| Peering, Parameter Store, IGW, SSM | 0 | — |
| **Total** | **~$0.11/hr** | |

**A four-hour session costs about $0.45.** A full day left running is ~$2.65; a forgotten week
is ~$18.50.

## The decision that drove this

The first draft of this project ran a `t4g.nano` NAT instance instead of a managed NAT gateway:

| | $/hr | 4-hour session | 30 days |
|---|---|---|---|
| NAT gateway | 0.045 | $0.18 | $32.85 |
| NAT instance `t4g.nano` | 0.0042 | $0.017 | $3.07 |

The monthly saving is real — about **$29** — and it is the number most cost-optimisation advice
quotes. But it only exists under 24/7 operation, which the teardown rule explicitly forbids.
Per session the difference is **16 cents**.

For those 16 cents the NAT instance introduced hand-rolled iptables masquerade, a disabled
`source_dest_check`, and rule persistence that had to survive reboots — the single most fragile
component in the build, sitting in the path of every demo. It was reverted.

**The lesson is about matching the optimisation to the usage pattern.** An hourly rate is the
right unit for a system that runs continuously; for one that runs four hours at a time, the
right unit is cost per session, and it inverts the answer. `01-eks-ordering-platform` keeps
`single_nat_gateway = true` for a different reason again: EKS needs the throughput and the
managed failover.

## What is deliberately not running

Both default to off and exist to be switched on briefly for evidence, then switched back:

| Toggle | Default | Cost when on | What it demonstrates |
|---|---|---|---|
| `multi_az` | `false` | +$0.016/hr | RDS standby in a second AZ |
| `desired_capacity` | `1` | +$0.0084/hr | ALB balancing across two AZs |

Running either permanently would not make the architecture more correct — the ASG and the
subnet groups span two AZs regardless. It would only cost more. Capture the console evidence
the original project never produced, then revert.

## Not costed here

RDS backups (`backup_retention_period = 0`), cross-AZ peering data transfer ($0.01/GB each way,
negligible at this data size but a genuine driver at production scale), and CloudWatch beyond
the free tier.

## Controls

- Account-wide `monthly-cost-guardrail` budget at $50, from `01`'s bootstrap stack.
- `scripts/teardown.sh` destroys both stacks and then checks the account for survivors.

The budget alone is not enough: at ~$2.65/day a forgotten stack takes about 19 days to trip a
$50 threshold. **The teardown script is the actual control.**
