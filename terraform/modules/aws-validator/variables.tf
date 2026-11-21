variable "network" {
  type        = string
  description = "Midnight network name (devnet, testnet, mainnet)"
}

variable "operator_role" {
  type        = string
  description = "Validator operator model (fno for Federated Node Operator, spo for Stake Pool Operator)"
  validation {
    condition     = contains(["fno", "spo"], var.operator_role)
    error_message = "operator_role must be either 'fno' or 'spo'."
  }
}

variable "vpc_cidr" {
  type        = string
  description = "VPC CIDR block"
  default     = "10.42.0.0/16"
}

variable "availability_zones" {
  type        = list(string)
  description = "Two availability zones for redundant P2P NLB listeners"
}

variable "ami_id" {
  type        = string
  description = "Hardened Ubuntu 24.04 or NixOS AMI ID"
}

variable "instance_type" {
  type        = string
  description = "EC2 instance type sized for Midnight node + optional Cardano partner-chain follower"
  default     = "c6i.2xlarge"
}

variable "chain_volume_gb" {
  type        = number
  description = "Encrypted gp3 EBS chain-state size in GiB"
  default     = 500
}

variable "ops_cidr_blocks" {
  type        = list(string)
  description = "Internal WireGuard/bastion CIDRs permitted to reach JSON-RPC 9944"
  default     = ["10.42.0.0/16"]
}

variable "monitoring_cidr_blocks" {
  type        = list(string)
  description = "Prometheus scraper CIDRs permitted to reach metrics port 9615"
  default     = ["10.42.0.0/16"]
}

variable "tags" {
  type        = map(string)
  description = "Additional resource tags"
  default     = {}
}
