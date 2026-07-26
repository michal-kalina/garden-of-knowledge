#!/bin/bash
# Runs once, as root, on first boot (EC2 user_data). Installs k3s — a single
# CNCF-conformant Kubernetes binary bundling containerd, Traefik (ingress),
# and local-path-provisioner (persistent volumes) — with no separate control
# plane fee, unlike EKS. See deploy/terraform/README.md and ADR-0009.
set -euo pipefail

exec > >(tee /var/log/k3s-install.log) 2>&1
echo "=== k3s install starting: $(date -u) ==="

# --write-kubeconfig-mode 644 so ec2-user can read it without sudo — this
# node has no other tenants, so there's no isolation this trades away.
curl -sfL https://get.k3s.io | sh -s - \
  --write-kubeconfig-mode 644 \
  --disable traefik=false

# Wait for the node to actually report Ready before declaring done — k3s
# returns from the install script before the kubelet has finished its first
# sync, and the README's "poll for k3s-install-done" step relies on this
# file meaning "you can kubectl get nodes and it'll say Ready," not just
# "the binary is on disk."
for i in $(seq 1 60); do
  if k3s kubectl get nodes 2>/dev/null | grep -q ' Ready'; then
    break
  fi
  sleep 5
done

mkdir -p /home/ec2-user/.kube
cp /etc/rancher/k3s/k3s.yaml /home/ec2-user/.kube/config
chown -R ec2-user:ec2-user /home/ec2-user/.kube

echo "=== k3s install finished: $(date -u) ===" | tee /var/log/k3s-install-done
