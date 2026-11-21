terraform {
  required_version = ">= 1.6.0"

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }
}

locals {
  name_prefix = "validator-${var.network}-${var.operator_role}"
}

resource "google_compute_network" "vpc" {
  name                    = "${local.name_prefix}-vpc"
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "validator" {
  name                     = "${local.name_prefix}-subnet"
  ip_cidr_range            = var.subnet_cidr
  region                   = var.region
  network                  = google_compute_network.vpc.id
  private_ip_google_access = true
}

resource "google_compute_firewall" "p2p_ingress" {
  name    = "${local.name_prefix}-allow-p2p-30333"
  network = google_compute_network.vpc.name

  direction     = "INGRESS"
  source_ranges = ["0.0.0.0/0"]
  target_tags   = ["validator-ops"]

  allow {
    protocol = "tcp"
    ports    = ["30333"]
  }
}

resource "google_compute_firewall" "internal_ops_metrics" {
  name    = "${local.name_prefix}-allow-internal-rpc-metrics"
  network = google_compute_network.vpc.name

  direction     = "INGRESS"
  source_ranges = var.ops_monitoring_cidrs
  target_tags   = ["validator-ops"]

  allow {
    protocol = "tcp"
    ports    = ["9615", "9944"]
  }
}

resource "google_kms_key_ring" "validator" {
  name     = "${local.name_prefix}-keyring"
  location = var.region
}

resource "google_kms_crypto_key" "chain_state" {
  name            = "${local.name_prefix}-chain-cmek"
  key_ring        = google_kms_key_ring.validator.id
  rotation_period = "7776000s"
}

resource "google_compute_disk" "chain_state" {
  name = "${local.name_prefix}-chain-state"
  type = "pd-ssd"
  zone = var.zone
  size = var.chain_disk_gb

  disk_encryption_key {
    kms_key_self_link = google_kms_crypto_key.chain_state.id
  }

  labels = {
    project  = "substrate-validator-ops"
    network  = var.network
    operator = var.operator_role
  }
}

resource "google_service_account" "validator" {
  account_id   = substr("${local.name_prefix}-sa", 0, 28)
  display_name = "Substrate Validator Service Account (${var.network})"
}

resource "google_secret_manager_secret" "validator_keystore" {
  secret_id = "${local.name_prefix}-session-keystore"

  replication {
    auto {}
  }
}

resource "google_compute_instance_template" "validator" {
  name_prefix  = "${local.name_prefix}-tpl-"
  machine_type = var.machine_type
  tags         = ["validator-ops", var.operator_role]

  shielded_instance_config {
    enable_secure_boot          = true
    enable_vtpm                 = true
    enable_integrity_monitoring = true
  }

  disk {
    source_image = var.source_image
    auto_delete  = true
    boot         = true
    disk_type    = "pd-ssd"
    disk_size_gb = 60

    disk_encryption_key {
      kms_key_self_link = google_kms_crypto_key.chain_state.id
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.validator.id
  }

  service_account {
    email  = google_service_account.validator.email
    scopes = ["cloud-platform"]
  }

  metadata = {
    enable-oslogin     = "TRUE"
    chain-network      = var.network
    operator-role      = var.operator_role
    keystore-secret-id = google_secret_manager_secret.validator_keystore.secret_id
    block-project-ssh  = "TRUE"
  }

  lifecycle {
    create_before_destroy = true
  }
}
