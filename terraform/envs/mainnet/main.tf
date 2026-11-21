terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region                      = var.aws_region
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
}

provider "google" {
  project = var.gcp_project
  region  = var.gcp_region
}

# Decentralised Stake Pool Operator (SPO) Mainnet topology across AWS + GCP.
module "aws_spo_validator" {
  source = "../../modules/aws-validator"

  network            = "mainnet"
  operator_role      = "spo"
  availability_zones = ["${var.aws_region}a", "${var.aws_region}b"]
  ami_id             = var.aws_ami_id
  instance_type      = "c6i.4xlarge"
  chain_volume_gb    = 1000
}

module "gcp_spo_standby" {
  source = "../../modules/gcp-validator"

  network       = "mainnet"
  operator_role = "spo"
  region        = var.gcp_region
  zone          = "${var.gcp_region}-b"
  machine_type  = "c2-standard-16"
  chain_disk_gb = 1000
}
