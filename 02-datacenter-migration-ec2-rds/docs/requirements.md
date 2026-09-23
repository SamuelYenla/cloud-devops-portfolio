# Requirements — 02 Data Center Migration to EC2 + RDS

Written before any Terraform. This is the contract the build is checked against: every resource
listed here must exist when the project is applied, and every applied resource must appear here.

---

## 1. Objective

Migrate a Flask wiki application and its MySQL database from a corporate data center to AWS,
using EC2 for the application tier and RDS for the database. The migration follows a
**rehost / lift-and-shift** model.

Because no real data center is available, the source environment is **simulated** as a second
VPC with its own CIDR, joined to the target VPC by VPC peering. This makes the migration real
and demonstrable — data actually moves across a network boundary, the cutover is timed, and the
rollback is exercised — rather than asserted.

This is a **portfolio artifact**, not a production system. It is built to be read, demoed
briefly, and destroyed. Where the cheapest option differs from what production would do, the
code carries a comment and [cost.md](cost.md) explains the tradeoff.

---

## 2. Source workload specification

Transcribed as text from the original sizing spreadsheet, so the spec no longer exists only
inside a screenshot.

### 2.1 On-premises hosts (as originally specified)

| Server | IP | Description | vCPU | RAM | Storage |
|---|---|---|---|---|---|
| `appserver01` | 10.0.0.20 | Python Web — Wiki Server Application | 1 | 1 GB | 4 GB |
| `dbserver01` | 10.0.0.31 | MySQL 5.7 — Wiki DB Server | 1 | 1 GB | 2 GB |

### 2.2 Software prerequisites

| Host | Type | Prerequisite | Command |
|---|---|---|---|
| `dbserver01` | Software | MySQL 5.7 | — |
| `dbserver01` | Storage | 4 GB | — |
| `appserver01` | OS | Ubuntu | — |
| `appserver01` | Software | Python 3.5+ | `apt-get install python3` |
| `appserver01` | OS package | python3-dev | `apt-get install python3-dev` |
| `appserver01` | OS package | libmysqlclient-dev | `apt-get install libmysqlclient-dev` |
| `appserver01` | OS package | libdev packages | `apt-get install libpq-dev python-dev libxml2-dev libxslt1-dev libldap2-dev libsasl2-dev libffi-dev` |
| `appserver01` | Software | PIP3 | `curl -O https://bootstrap.pypa.io/get-pip.py ; python3 get-pip.py --user` |
| `appserver01` | Python lib | flask | `pip3 install flask` |
| `appserver01` | Python lib | wtforms | `pip3 install wtforms` |
| `appserver01` | Python lib | flask_mysqldb | `pip3 install flask_mysqldb` |
| `appserver01` | Python lib | passlib | `pip3 install passlib` |

### 2.3 Deviations from the original spec, and why

| Original | This build | Reason |
|---|---|---|
| On-prem `10.0.0.0/24`, AWS VPC `10.0.0.0/16` | DC `10.10.0.0/16`, AWS `10.20.0.0/16` | The original ranges **overlap**. No VPN, Direct Connect or peering can join overlapping CIDRs — which is why the original's VPC console showed `Network connections (0)`. Non-overlapping ranges are a precondition for the migration, not a preference. |
| Two source hosts (`appserver01`, `dbserver01`) | One `t3.micro` running both, MySQL in Docker | Saves an instance. Changes nothing about the migration: the dump still crosses the peering link to RDS. MySQL 5.7 ships as a container because 5.7 packages are awkward on current Ubuntu. |
| App on EC2 in a **public** subnet with an ephemeral public IP | App in **private** subnets behind an ALB | The original exposed the app server directly and its public IP changed on every stop/start. |
| `t2.micro` (previous generation) | `t3.micro` / `t4g.micro` | t2 is previous-gen and more expensive for less performance. |
| MySQL 5.7 on RDS | MySQL 8.0 on RDS | 5.7 is past RDS standard support. The **source** stays 5.7 — that is the point of a version-upgrading migration. |
| "Multiple AZs for high availability" (single instance, no ALB/ASG) | ALB + ASG across 2 AZs; Multi-AZ RDS available via toggle | The original claimed HA it did not build. |

---

## 3. Target architecture

```
                         Internet
                             │
                    ┌────────▼────────┐
                    │  ALB (public)   │  10.20.0.0/16
                    └────────┬────────┘
              ┌──────────────┴──────────────┐
         ┌────▼─────┐                  ┌────▼─────┐   private subnets
         │ app (ASG)│                  │ app (ASG)│   (no public IP)
         └────┬─────┘                  └────┬─────┘
              └──────────────┬──────────────┘
                        ┌────▼────┐              ┌──────────────┐
                        │   RDS   │              │ NAT gateway  │→ IGW
                        │ MySQL 8 │              │   (single)   │
                        └─────────┘              └──────────────┘
                       database subnets

     ══════════════ VPC peering ══════════════

                    10.10.0.0/16  (simulated DC)
                    ┌──────────────────────────┐
                    │ dc-host (t3.micro)       │
                    │  Flask wiki + MySQL 5.7  │
                    └──────────────────────────┘
```

---

## 4. Resource inventory

Region `us-east-1`. Name prefix `dc-migration` (`var.name`). All resources tagged
`Project=cloud-devops-portfolio`, `Component=<stack>`, `ManagedBy=terraform` via provider
`default_tags`.

### 4.1 `terraform/datacenter/` — simulated source

| Resource | Spec | Why |
|---|---|---|
| VPC | `10.10.0.0/16` | Source network; must not overlap the target |
| Public subnet × 1 | `10.10.0.0/24` | Single AZ is sufficient for a source being decommissioned |
| Internet gateway | — | Package installs during bootstrap |
| Route table + association | default → IGW | — |
| Security group | 22 (SSM only), 80 from own VPC, 3306 from `10.20.0.0/16` | 3306 from the target VPC is what the migration dump traverses |
| EC2 instance | `t3.micro`, Ubuntu 22.04, 8 GB gp3 | Runs Flask wiki + MySQL 5.7 (Docker) |
| IAM role + instance profile | `AmazonSSMManagedInstanceCore` | Session Manager access; no SSH key |

Bootstrap seeds the wiki schema and ~500 rows so the migration has verifiable content.

### 4.2 `terraform/aws/` — target environment

Network, database and application tiers in one stack. They are created and destroyed together
in every session, so splitting them would add `init`/`apply`/`destroy` cycles and
`terraform_remote_state` plumbing without buying any lifecycle independence. `01` splits its
stacks because its state bucket carries `prevent_destroy` and genuinely does have a different
lifecycle.

#### Network — `terraform-aws-modules/vpc/aws ~> 6.0`

| Resource | Spec | Why |
|---|---|---|
| VPC | `10.20.0.0/16`, DNS hostnames + support on | RDS endpoint resolution requires both |
| Public subnets × 2 | `10.20.0.0/24`, `10.20.1.0/24` | ALB requires ≥ 2 AZs |
| Private subnets × 2 | `10.20.10.0/24`, `10.20.11.0/24` | App tier, no public IPs |
| Database subnets × 2 | `10.20.20.0/24`, `10.20.21.0/24` | RDS subnet group requires ≥ 2 AZs |
| DB subnet group | `create_database_subnet_group = true` | Module-provided; no hand-rolled group |
| Internet gateway | — | ALB and NAT egress |
| NAT gateway | `single_nat_gateway = true` | A `t4g.nano` NAT instance would save ~$29/month, but only under 24/7 operation. Against the teardown rule the managed gateway costs ~$0.16 more **per session** — not enough to justify hand-rolled iptables masquerade as a dependency of every demo. See [cost.md](cost.md). |

#### Peering

| Resource | Spec | Why |
|---|---|---|
| VPC peering connection | DC ↔ target, auto-accept (same account/region) | The connection the original never had |
| Route: target private → `10.10.0.0/16` | — | Outbound to the source |
| Route: DC public → `10.20.0.0/16` | — | Return path, written into the DC stack's route table |

Free within a region. Cross-AZ peering transfer is $0.01/GB each way — negligible at this data
size, but a real cost driver at production scale.

#### Database

| Resource | Spec | Why |
|---|---|---|
| `aws_db_instance` | MySQL 8.0, `db.t4g.micro`, 20 GB gp3 | 20 GB is the RDS minimum. Source is 5.7; the upgrade is part of the migration. |
| | `storage_encrypted = true` | Free; no reason not to |
| | `backup_retention_period = 0` | Nothing here is worth retaining; avoids snapshot storage cost |
| | `multi_az = var.multi_az` (**default `false`**) | Multi-AZ doubles the RDS bill for a property no demo exercises. Flip to `true` for one apply, capture evidence of the standby, revert. |
| | `skip_final_snapshot = true`, `deletion_protection = false` | Teardown rule |
| `random_password` | 32 chars, no specials | Avoids quoting problems in the bootstrap shell |
| SSM Parameter `/dc-migration/db/password` | `SecureString` | **Parameter Store standard tier is free; Secrets Manager is $0.40/secret/month.** Also deletes immediately — Secrets Manager's 7-day soft delete would collide with the next apply. |
| SSM Parameter `/dc-migration/db/host` | `String`, `ignore_changes = [value]` | **The cutover switch.** Starts at the DC host, moves to the RDS endpoint. Terraform stops managing the value because `migrate.sh` writes it. |
| Security group | 3306 **from the app SG by reference**, not CIDR | A CIDR rule would admit anything that lands in the subnet |

#### Application tier

| Resource | Spec | Why |
|---|---|---|
| ALB | internet-facing, public subnets, HTTP :80 | Largest cost line (~43%) but carries the private-subnet and multi-AZ story |
| Target group | HTTP :8000, health check `/healthz` | `/healthz` tests the DB connection, so an instance that cannot reach its database is replaced rather than counted healthy |
| Launch template | `t4g.micro`, Ubuntu 22.04 arm64, IMDSv2 required, no public IP | — |
| Auto Scaling group | `min 1`, `max 2`, `desired = var.desired_capacity` (**default 1**), `health_check_type = "ELB"` | Bump to 2 to capture cross-AZ balancing, then revert. The ASG proves the HA claim; running two instances permanently does not make it more true. |
| IAM role + instance profile | `AmazonSSMManagedInstanceCore`, plus `ssm:GetParameter` on **the two parameters** and `kms:Decrypt` scoped by `kms:ViaService` | Least privilege: two named parameters, not `ssm:*` |
| Security group (app) | 8000 from the ALB SG only | A public IP would not make an instance reachable even if one were attached |
| Security group (ALB) | 80 from `0.0.0.0/0` | — |

`user_data` installs the §2.2 package list and writes `/opt/wiki/start.sh`, which reads both SSM
parameters at startup and execs gunicorn. Reading the host at startup is what makes cutover a
parameter change plus a service restart rather than a new launch template version and an
instance refresh.

### 4.6 Reused, not created

| Resource | Where |
|---|---|
| State bucket `tfstate-188050967390-us-east-1` | Account-wide, from `01`'s bootstrap. Key prefix `02-datacenter-migration-ec2-rds/<stack>/` |
| Budget `monthly-cost-guardrail` ($50) | Account-wide, from `01`'s bootstrap |

**02 has no bootstrap stack of its own.**

---

## 5. Process

Mapped onto the four phases from the original project.

### Phase 1 — Planning
Sizing (§2), prerequisites (§2.2), naming, CIDR allocation, cost model ([cost.md](cost.md)).
Deliverable: this document.

### Phase 2 — Implementation

Two stacks. `datacenter` is applied first and destroyed last; `aws` reads its outputs through a
single `terraform_remote_state` data source.

| # | Stack | Consumes | Contents |
|---|---|---|---|
| 1 | `datacenter` | — | Source VPC and host |
| 2 | `aws` | `datacenter`: `vpc_id`, `vpc_cidr`, `route_table_id`, `host_private_ip` | Target VPC, peering, RDS, ALB + ASG |

Everything inside `aws` is ordered by Terraform's own dependency graph rather than by hand.

### Phase 3 — Go-Live
1. **Dry run** — `mysqldump` from the DC host over peering, restore to RDS, compare row counts
   and checksums, DC stays live. Records the cutover window duration.
2. **Cutover** — stop writes at the DC, final dump, restore, bump the launch template with the
   RDS endpoint, ASG instance refresh.
3. **Rollback** — revert the launch template to the previous version, re-point at the DC host.
   Valid until DC decommission.

Validation checklist lives in [runbook.md](runbook.md).

### Phase 4 — Post Go-Live
Confirm the app serves from RDS, instances carry no public IPs and are reachable only via SSM,
CloudWatch shows traffic through the ALB. Then `scripts/teardown.sh`.

---

## 6. Out of scope

Stated explicitly so their absence reads as a decision, not an oversight.

- **VPN / Direct Connect.** VPC peering stands in for data-center connectivity. A real migration
  would use Site-to-Site VPN or Direct Connect; both cost money continuously and neither changes
  what the migration demonstrates.
- **AWS DMS.** `mysqldump` suits a 2 GB database. DMS earns its place at continuous replication
  and near-zero-downtime cutover, neither of which this workload needs.
- **Production HA by default.** Multi-AZ RDS and a 2-instance ASG are toggles, exercised for
  evidence then reverted. See §4.4 and §4.5.
- **CI/CD.** `01` covers pipelines. Adding them here would duplicate that without adding to the
  migration story.
- **HTTPS / ACM / Route 53.** A certificate needs a domain; the ALB's DNS name is sufficient for
  a demo.
- **Automated backups, monitoring stack, WAF.** Not what this project is demonstrating.

---

## 7. Acceptance criteria

The build is done when all of these hold:

1. `terraform fmt -check` and `terraform validate` pass in all five stacks.
2. All five stacks apply cleanly in dependency order.
3. The DC host serves the wiki and its MySQL 5.7 holds the seed rows.
4. A private-subnet host reaches the internet **through the NAT gateway**.
5. `mysql -h <dc-host>` succeeds from the target VPC — the link the original never had.
6. The ALB serves the wiki; app instances have **no public IPs** and are reachable only via SSM.
7. `migrate.sh` dry run: row counts and checksums match.
8. After cutover, a page created through the ALB lands in **RDS**, not the DC host.
9. Rollback is exercised at least once. An untested rollback is not a rollback.
10. `scripts/teardown.sh` reports zero surviving resources.
11. **Every resource in §4 exists after apply, and every applied resource appears in §4.**
    A mismatch means one of the two is wrong — which is exactly the defect the original
    shipped with (its sizing sheet said AZ `1a`; the running instance was in `1d`).
