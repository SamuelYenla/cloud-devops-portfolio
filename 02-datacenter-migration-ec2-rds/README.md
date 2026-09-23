# 02 - Data Center Migration to EC2 + RDS

Migrating a Flask wiki and its MySQL database from a corporate data center to AWS, using EC2 for
the application tier and RDS for the database.

The data center is **simulated as a second VPC** with a non-overlapping CIDR, joined to the
target by VPC peering. That makes the migration real and demonstrable: data crosses a network
boundary, the cutover is timed, and the rollback is exercised.

Start with [docs/requirements.md](docs/requirements.md) - full resource inventory, process, and
what is deliberately out of scope.

## Layout

| Path | Contents |
|---|---|
| `terraform/datacenter/` | Simulated source: VPC + one host running Flask and MySQL 5.7 |
| `terraform/network/` | Target VPC: public / private / database subnets, NAT instance |
| `terraform/peering/` | VPC peering and routes between the two |
| `terraform/data/` | RDS MySQL 8.0 |
| `terraform/compute/` | ALB + ASG running the wiki |
| `app/wiki/` | The application being migrated |
| `scripts/` | `migrate.sh` (dry run / cutover / rollback), `teardown.sh` |
| `docs/` | `requirements.md`, `architecture.md`, `runbook.md`, `cost.md` |

Apply in the order listed; destroy in reverse. Each stack reads the previous ones' outputs via
`terraform_remote_state`.

## Cost

About **$0.07/hour**, so a four-hour session costs roughly **$0.29**. The ALB is the largest
single item at 43%. See [docs/cost.md](docs/cost.md) for the breakdown and the two tradeoffs
that keep it there: a `t4g.nano` NAT instance instead of a NAT gateway, and Multi-AZ RDS behind
a toggle rather than on by default.

> Run `scripts/teardown.sh` at the end of every session. A forgotten `destroy` costs ~$12/week,
> and the account-wide $50 budget alert would not fire until day 29.

## Status

| Stack | State |
|---|---|
| `datacenter` | Written, validates |
| `network` | Written, validates |
| `peering` | Not started |
| `data` | Not started |
| `compute` | Not started |

Nothing has been applied to AWS yet.
