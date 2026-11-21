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

# Federated Node Operator (FNO) Testnet topology spanning AWS + GCP.
module "aws_fno_validator" {
  source = "../../modules/aws-validator"

  network            = "testnet"
  operator_role      = "fno"
  availability_zones = ["${var.aws_region}a", "${var.aws_region}b"]
  ami_id             = var.aws_ami_id
  chain_volume_gb    = 300
}

module "gcp_fno_sentry" {
  source = "../../modules/gcp-validator"

  network       = "testnet"
  operator_role = "fno"
  region        = var.gcp_region
  zone          = "${var.gcp_region}-b"
  chain_disk_gb = 300
}
