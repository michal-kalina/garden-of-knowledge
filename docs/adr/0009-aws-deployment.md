# ADR-0009: k3s on one EC2 instance; EKS only as a short-lived exercise

**Status:** accepted · **Date:** 2026-07

## Context

Phase 5 puts the project on AWS. I gave myself a hard budget of $80/month
and priced two variants before writing any Terraform.

**Variant A — "textbook" EKS.** Managed control plane, worker nodes in
private subnets behind a NAT Gateway, an ALB in front, RDS for Postgres:

| Component | Monthly (24/7) |
|---|---|
| EKS control plane | ~$73 ($0.10/h) |
| NAT Gateway, 1 AZ | ~$33 + $0.045/GB |
| Application Load Balancer | ~$16–20 |
| 2× t3.small workers | ~$30 |
| RDS db.t4g.micro | ~$12 |
| **Total** | **~$164–168** |

That's double the budget before a single pod runs. The control plane fee
alone is 91% of the $80, and it bills every hour whether anyone visits the
site or not. Same for NAT and the ALB. This is where the discussion with
variant A basically ended.

**Variant B — k3s on a single EC2 instance.** One t4g.small in the default
VPC with a public IP and a tight security group. Postgres+pgvector and
MinIO run as pods on the same node instead of RDS/S3. Total lands around
$15–17/month (breakdown in `deploy/terraform/README.md`).

## Decision

Variant B for the always-on deployment.

k3s is CNCF-certified Kubernetes, so everything in `deploy/k8s/` stays
plain Kubernetes YAML and would run on EKS unchanged — the only thing
that differs is the kubeconfig. What variant A buys — an HA control
plane, private networking, managed Postgres — solves problems this
deployment doesn't have: nobody's uptime depends on it and the data is my
own test documents.

I still want EKS + Terraform demonstrably in the repo, though, so that
part becomes a separate ephemeral module (`deploy/eks-exercise/`): apply,
deploy the app, take the screenshots, destroy the same day. One day of
EKS is about $3; a month is $73 before anything else. Same skill shown,
a fraction of the price. The module keeps its own directory and its own
state, specifically so it can never be applied together with the
always-on stack by accident.

## How I'm spending the budget

- **t4g.small on-demand, ~$12/month.** Elastic IP attached, so the address
  survives a stop/start — and I *will* be stopping the instance between
  demo sessions, because compute is the biggest line item and it only
  bills while running.
- **Default VPC, no NAT Gateway.** Outbound traffic (image pulls, calls to
  Voyage/Anthropic/OpenRouter) goes straight out; inbound is limited by
  the security group to my IP for SSH/kubectl and 80/443 for the app. For
  a single-tenant node, NAT would cost $33/month and change nothing I
  actually care about.
- **Postgres and MinIO as pods on the node**, not RDS/S3 — saves another
  ~$12+/month and one more moving part.
- **AWS Budgets alert at $50** (`budget.tf`, email at 80%). Early warning,
  not a cap — AWS has no real spending cap, so the alert plus checking the
  bill is the honest version.

What's left of the $80 covers the EKS exercise days and mistakes. There
will be mistakes.

## Trade-offs I'm accepting

Single node means no HA — the instance dies, the app is down. Worse:
because Postgres runs on that node, the instance dying loses *data*, not
just availability. For a portfolio deployment that's fine; for anything
with real users it wouldn't be, and RDS would be the first thing back on
the table.

Terraform state stays local (`terraform.tfstate`, gitignored). One
operator, one laptop. A team would need S3 + DynamoDB locking — it's a
known upgrade path, noted in the Terraform README, not built, because
nothing here needs it yet.
