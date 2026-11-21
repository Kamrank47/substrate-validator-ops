variable "aws_region" {
  type    = string
  default = "eu-central-1"
}

variable "aws_ami_id" {
  type    = string
  default = "ami-0123456789abcdef0"
}

variable "gcp_project" {
  type    = string
  default = "validator-mainnet-ops"
}

variable "gcp_region" {
  type    = string
  default = "europe-west3"
}
