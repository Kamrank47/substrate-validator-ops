# Deliberately misconfigured validator IaC fixture used to prove the
# Conftest/OPA security gate fails on unsafe deployments.

variable "validator_session_key_mnemonic" {
  type    = string
  default = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
}

resource "aws_security_group" "unsafe_validator" {
  name        = "unsafe-substrate-validator-sg"
  description = "EXPOSED ports fixture for policy testing"

  ingress {
    from_port   = 9944
    to_port     = 9944
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 9615
    to_port     = 9615
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_ebs_volume" "unencrypted_chain" {
  availability_zone = "eu-west-1a"
  size              = 500
  encrypted         = false
}

resource "aws_launch_template" "weak_imdsv1" {
  name_prefix   = "unsafe-validator-"
  instance_type = "c6i.2xlarge"

  metadata_options {
    http_tokens = "optional"
  }
}

resource "google_compute_firewall" "unsafe_rpc" {
  name          = "unsafe-expose-rpc"
  network       = "default"
  source_ranges = ["0.0.0.0/0"]

  allow {
    protocol = "tcp"
    ports    = ["9944"]
  }
}

resource "google_compute_disk" "unencrypted_gcp_chain" {
  name = "unsafe-chain-disk"
  type = "pd-ssd"
  zone = "europe-west1-b"
  size = 500
}
