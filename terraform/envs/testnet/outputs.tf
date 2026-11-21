output "aws_fno_p2p_endpoint" {
  value = "${module.aws_fno_validator.p2p_nlb_dns}:30333"
}

output "gcp_fno_disk_id" {
  value = module.gcp_fno_sentry.gcp_chain_disk_id
}
