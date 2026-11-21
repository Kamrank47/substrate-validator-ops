output "aws_spo_p2p_endpoint" {
  value = "${module.aws_spo_validator.p2p_nlb_dns}:30333"
}

output "gcp_spo_standby_disk_id" {
  value = module.gcp_spo_standby.gcp_chain_disk_id
}
