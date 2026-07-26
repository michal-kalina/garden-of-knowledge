variable "aws_region" {
  description = "AWS region to deploy into."
  type        = string
  default     = "eu-central-1"
}

variable "project_name" {
  description = "Prefix applied to every resource name and tag."
  type        = string
  default     = "gok"
}

variable "instance_type" {
  description = <<-EOT
    EC2 instance type for the single-node k3s host. t4g.small (2 vCPU, 2GB
    RAM, Graviton/ARM64 — cheaper per hour than the x86 t3 equivalent) is
    the tested default; t4g.medium (4GB RAM) is a safer choice if you plan
    to run the full stack (postgres, minio, parser, api, worker, web) with
    headroom rather than trimming resource requests.
  EOT
  type        = string
  default     = "t4g.small"
}

variable "root_volume_gb" {
  description = "Root EBS volume size in GiB (gp3). 20 is enough for the OS, k3s, container images, and a small Postgres dataset — raise this before it fills up, not after."
  type        = number
  default     = 20
}

variable "admin_cidr" {
  description = <<-EOT
    CIDR allowed to reach SSH (22) and the k3s API (6443) — your own IP,
    not 0.0.0.0/0. Find yours with `curl -s ifconfig.me` and pass it as
    "YOUR.IP.HERE/32". Required — there is deliberately no default, so a
    forgotten -var doesn't silently open the cluster API to the internet.
  EOT
  type        = string
}

variable "ssh_public_key" {
  description = "Contents of your SSH public key (e.g. `cat ~/.ssh/id_ed25519.pub`), used to create the EC2 key pair."
  type        = string
}

variable "budget_limit_usd" {
  description = "Monthly AWS Budgets alert threshold, in USD. Deliberately below the $80 training budget so alerts fire with room to react."
  type        = number
  default     = 50
}

variable "budget_alert_email" {
  description = "Email address for AWS Budgets threshold notifications."
  type        = string
}
