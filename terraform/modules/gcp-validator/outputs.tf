output "gcp_vpc_id" {
  description = "GCP validator VPC ID"
  value       = google_compute_network.vpc.id
}

output "gcp_chain_disk_id" {
  description = "CMEK-encrypted PD-SSD chain disk ID"
  value       = google_compute_disk.chain_state.id
}

output "gcp_keystore_secret_id" {
  description = "GCP Secret Manager ID for Midnight session keystore"
  value       = google_secret_manager_secret.validator_keystore.secret_id
}
