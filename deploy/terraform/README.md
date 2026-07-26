# AWS infrastructure: k3s on a single EC2 host

One EC2 instance running [k3s](https://k3s.io). This is the always-on
deployment target; the manifests that actually run on it live in
`deploy/k8s/`. Why k3s on one box and not EKS: the EKS control plane
alone costs ~$73/month against my $80 budget — full comparison in
[ADR-0009](../../docs/adr/0009-aws-deployment.md).

## What it costs

| Item | Monthly |
|---|---|
| EC2 t4g.small (on-demand, 24/7) | ~$12 |
| EBS gp3 20GB | ~$1.60 |
| Elastic IP (attached to a running instance) | $0 |
| Data transfer at portfolio-demo traffic levels | ~$1–2 |
| **Total** | **~$15–17** |

`budget.tf` adds an AWS Budgets alert: email at 80% of a $50 threshold.
It's a smoke detector, not a cap — AWS doesn't do hard spending caps.

## Prerequisites

- AWS credentials configured locally (`aws configure` or `AWS_PROFILE`;
  the module doesn't set credentials itself)
- Terraform >= 1.7
- An SSH key pair (`ssh-keygen -t ed25519 -C "gok-deploy"`)
- Your current public IP for `admin_cidr`: `curl -s ifconfig.me`

## Apply

```bash
cd deploy/terraform
cp terraform.tfvars.example terraform.tfvars
# fill in: admin_cidr, ssh_public_key, budget_alert_email

terraform init
terraform validate
terraform plan
terraform apply
```

## First boot

k3s installs itself via `user_data`, which takes another minute or two
after the instance becomes SSH-reachable. The outputs cover the waiting
and the kubeconfig fetch:

```bash
terraform output -raw wait_for_k3s_command | bash
eval "$(terraform output -raw fetch_kubeconfig_command)"
kubectl get nodes   # one node, Ready
```

`kubeconfig.yaml` is cluster-admin for this node and is gitignored.

## Stopping the instance between sessions

Nothing here has an SLA, so when I'm not demoing, the instance is
stopped — EC2 bills for hours *running*, and compute is most of the bill:

```bash
aws ec2 stop-instances --instance-ids $(terraform output -raw instance_id)
# later
aws ec2 start-instances --instance-ids $(terraform output -raw instance_id)
```

The Elastic IP stays attached across stop/start, so the public IP (and
any DNS record) doesn't change. EBS and the allocated EIP keep their
small charges while stopped; only the compute portion goes away.

## Destroy

```bash
terraform destroy
```

Removes the instance, security group, key pair, EIP and the budget
alert. Everything deployed via `deploy/k8s/` disappears with the node —
including the data in Postgres and MinIO, permanently. Back up first if
that matters.

## State

State is local (`terraform.tfstate`, gitignored) — deliberate, one
operator. The upgrade path for a team is an S3 backend with DynamoDB
locking; see [ADR-0009](../../docs/adr/0009-aws-deployment.md) for why
that's noted but not built.

## Next step

`deploy/k8s/`: namespace, Postgres with pgvector, MinIO, the parser
service, api, worker, web, and an Ingress — the application itself.
