# Single-file provider config. State is local on purpose (see README.md
# "State" section) — a remote S3+DynamoDB backend is the right call for a
# team, but adds a bootstrapping step this single-operator project doesn't
# need yet.
terraform {
  required_version = ">= 1.7"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
  }
}

provider "aws" {
  region = var.aws_region
}
