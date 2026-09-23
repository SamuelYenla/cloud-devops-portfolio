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
| `terraform/aws/` | Target VPC, peering, RDS MySQL 8.0, ALB + ASG |
| `app/wiki/` | The application being migrated |
| `scripts/` | `migrate.sh` (dry-run / cutover / rollback), `teardown.sh` |
| `docs/` | `requirements.md`, `cost.md` |

Two stacks, not five. Everything in `aws` is created and destroyed in the same session, so
Terraform's own dependency graph orders it — splitting would only add apply/destroy cycles and
remote-state plumbing. `01` splits its stacks because its state bucket has `prevent_destroy`
and a genuinely different lifecycle.

Apply `datacenter` then `aws`; destroy in reverse. `aws` reads the data center's outputs
through a single `terraform_remote_state` data source.

## Cost

About **$0.11/hour**, so a four-hour session costs roughly **$0.45**. The NAT gateway and ALB
are half of it. See [docs/cost.md](docs/cost.md) for the breakdown, the two toggles that stay
off by default (`multi_az`, `desired_capacity`), and why the first draft's NAT *instance* was
reverted — it saved $29/month but only 16 cents per session, for the most fragile component in
the build.

> Run `scripts/teardown.sh` at the end of every session. It destroys both stacks and then
> checks the account for survivors. At ~$2.65/day a forgotten stack takes ~19 days to trip the
> $50 budget alert, so the script is the real control, not the budget.

## Status

| Stack | State |
|---|---|
| `datacenter` | Written, validates |
| `aws` | Written, validates |
| `scripts/` | Written, syntax-checked |

Nothing has been applied to AWS yet, so no `user_data` script has run for real.
