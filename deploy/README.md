# deploy

- `terraform/` — AWS infrastructure: a single EC2 host running k3s.
  [ADR-0009](../docs/adr/0009-aws-deployment.md) explains why not EKS
  for the always-on part (short version: the control plane fee alone
  would eat the budget).
- `k8s/` — Kubernetes manifests that run on that host (next step).
- `eks-exercise/` — ephemeral EKS module: apply for a demo session,
  destroy the same day. Kept in its own directory with its own state so
  it can't be applied together with the always-on stack.
