variable "network" {
  type        = string
  description = "Substrate network name (devnet, testnet, mainnet)"
}

variable "operator_role" {
  type        = string
  description = "Validator role: fno or spo"
  validation {
    condition     = contains(["fno", "spo"], var.operator_role)
    error_message = "operator_role must be either 'fno' or 'spo'."
  }
}

variable "region" {
  type        = string
  description = "GCP region"
  default     = "europe-west1"
}

variable "zone" {
  type        = string
  description = "GCP zone"
  default     = "europe-west1-b"
}

variable "subnet_cidr" {
  type        = string
  description = "Subnet CIDR block"
  default     = "10.52.0.0/20"
}

variable "ops_monitoring_cidrs" {
  type        = list(string)
  description = "Internal CIDRs permitted to reach JSON-RPC 9944 and Prometheus 9615"
  default     = ["10.52.0.0/20"]
}

variable "machine_type" {
  type        = string
  description = "GCP compute machine type"
  default     = "c2-standard-8"
}

variable "source_image" {
  type        = string
  description = "Hardened Ubuntu LTS image family"
  default     = "ubuntu-os-cloud/ubuntu-2404-lts-amd64"
}

variable "chain_disk_gb" {
  type        = number
  description = "CMEK-encrypted PD-SSD chain disk size in GiB"
  default     = 500
}
