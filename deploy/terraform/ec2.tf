locals {
  common_tags = {
    Project   = var.project_name
    ManagedBy = "terraform"
  }
}

# Deliberately the account's default VPC, not a purpose-built one: the
# instance gets a public IP directly instead of sitting in a private subnet
# behind a NAT Gateway. A NAT Gateway alone runs ~$33/month just to exist,
# before any data flows through it — for a single-node portfolio deployment
# that cost buys nothing a tight security group doesn't already provide.
# See deploy/terraform/README.md and ADR-0009 for the full trade-off.
data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# Latest Amazon Linux 2023 AMI, ARM64 (Graviton) — resolved at apply time via
# SSM, so this never goes stale the way a hardcoded AMI ID would.
data "aws_ssm_parameter" "al2023_arm64" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-arm64"
}

resource "aws_key_pair" "admin" {
  key_name   = "${var.project_name}-admin"
  public_key = var.ssh_public_key
  tags       = local.common_tags
}

resource "aws_security_group" "k3s_host" {
  name        = "${var.project_name}-k3s-host"
  description = "Garden of Knowledge k3s host: SSH + k3s API restricted to admin_cidr, HTTP/HTTPS open for the public app"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "SSH (admin only)"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [var.admin_cidr]
  }

  ingress {
    description = "k3s / Kubernetes API (admin only — this is cluster-admin access, never open it to 0.0.0.0/0)"
    from_port   = 6443
    to_port     = 6443
    protocol    = "tcp"
    cidr_blocks = [var.admin_cidr]
  }

  ingress {
    description = "HTTP (public — Traefik redirects to HTTPS)"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "HTTPS (public)"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "All outbound (pulling images, OS updates, calling Voyage/Anthropic/OpenRouter APIs)"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.common_tags
}

resource "aws_instance" "k3s_host" {
  ami                         = data.aws_ssm_parameter.al2023_arm64.value
  instance_type               = var.instance_type
  subnet_id                   = tolist(data.aws_subnets.default.ids)[0]
  vpc_security_group_ids      = [aws_security_group.k3s_host.id]
  key_name                    = aws_key_pair.admin.key_name
  associate_public_ip_address = true

  root_block_device {
    volume_size           = var.root_volume_gb
    volume_type           = "gp3"
    delete_on_termination = true
    tags                  = local.common_tags
  }

  # Installs k3s and leaves a completion marker at /var/log/k3s-install-done
  # — poll for that file over SSH before trying to fetch the kubeconfig
  # (see README.md "First boot" section); k3s takes a minute or two to
  # actually come up after the instance itself is reachable.
  user_data                   = file("${path.module}/scripts/install-k3s.sh")
  user_data_replace_on_change = true

  tags = merge(local.common_tags, { Name = "${var.project_name}-k3s-host" })
}

# A stable IP that survives a stop/start (e.g. to save money overnight — see
# README.md). Free while attached to a running instance; AWS charges for an
# Elastic IP only when it's allocated but NOT attached to anything, so
# don't `terraform apply` with the instance stopped and this resource
# orphaned for long.
resource "aws_eip" "k3s_host" {
  domain   = "vpc"
  instance = aws_instance.k3s_host.id
  tags     = merge(local.common_tags, { Name = "${var.project_name}-eip" })
}
