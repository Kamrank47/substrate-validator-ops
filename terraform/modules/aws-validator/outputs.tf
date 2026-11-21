output "p2p_nlb_dns" {
  description = "Public NLB DNS exposing port 30333 only for Substrate libp2p peers"
  value       = aws_lb.p2p.dns_name
}

output "chain_volume_id" {
  description = "KMS-encrypted gp3 EBS chain state volume ID"
  value       = aws_ebs_volume.chain_state.id
}

output "keystore_secret_arn" {
  description = "Secrets Manager ARN holding Substrate validator session keys"
  value       = aws_secretsmanager_secret.validator_keystore.arn
}

output "vpc_id" {
  description = "AWS validator VPC ID"
  value       = aws_vpc.this.id
}
