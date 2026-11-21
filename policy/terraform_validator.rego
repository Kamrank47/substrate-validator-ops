package main

import rego.v1

# Enforce Substrate validator IaC security baselines on HCL2 Terraform/OpenTofu files.
# Blocks exposed RPC (9944), Prometheus (9615), SSH (22), unencrypted chain storage,
# missing IMDSv2 enforcement, and literal private keys/mnemonics in config.

restricted_ports := {22, 9615, 9944}

as_list(x) := x if {
	is_array(x)
} else := [x]

port_in_range(from_p, to_p, target) if {
	from_p <= target
	to_p >= target
}

# --- AWS Security Group inline ingress blocks ---
deny contains msg if {
	some res_name, res in input.resource.aws_security_group
	some ing in as_list(res.ingress)
	ing.cidr_blocks[_] == "0.0.0.0/0"
	some port in restricted_ports
	port_in_range(ing.from_port, ing.to_port, port)
	msg := sprintf("aws_security_group.%s: world-open 0.0.0.0/0 ingress covers restricted port %d (only P2P 30333 may be public)", [res_name, port])
}

# --- Standalone aws_security_group_rule ---
deny contains msg if {
	some res_name, res in input.resource.aws_security_group_rule
	res.type == "ingress"
	res.cidr_blocks[_] == "0.0.0.0/0"
	some port in restricted_ports
	port_in_range(res.from_port, res.to_port, port)
	msg := sprintf("aws_security_group_rule.%s: world-open 0.0.0.0/0 ingress covers restricted port %d", [res_name, port])
}

# --- GCP VPC Firewalls ---
gcp_rule_covers_port(rule, port) if {
	not rule.ports
}

gcp_rule_covers_port(rule, port) if {
	some p in rule.ports
	to_number(p) == port
}

gcp_rule_covers_port(rule, port) if {
	some p in rule.ports
	contains(p, "-")
	parts := split(p, "-")
	to_number(parts[0]) <= port
	to_number(parts[1]) >= port
}

deny contains msg if {
	some res_name, res in input.resource.google_compute_firewall
	res.source_ranges[_] == "0.0.0.0/0"
	some rule in as_list(res.allow)
	some port in restricted_ports
	gcp_rule_covers_port(rule, port)
	msg := sprintf("google_compute_firewall.%s: world-open 0.0.0.0/0 rule exposes restricted port %d", [res_name, port])
}

# --- Chain State Volume Encryption (AWS EBS & GCP Persistent Disk) ---
deny contains msg if {
	some res_name, res in input.resource.aws_ebs_volume
	res.encrypted != true
	msg := sprintf("aws_ebs_volume.%s: chain-state EBS volume must set encrypted = true", [res_name])
}

deny contains msg if {
	some res_name, res in input.resource.google_compute_disk
	not res.disk_encryption_key
	msg := sprintf("google_compute_disk.%s: GCP chain-state disk must configure disk_encryption_key (CMEK)", [res_name])
}

# --- EC2 Launch Template IMDSv2 Enforcement ---
deny contains msg if {
	some res_name, res in input.resource.aws_launch_template
	not res.metadata_options
	msg := sprintf("aws_launch_template.%s: missing metadata_options to enforce IMDSv2", [res_name])
}

deny contains msg if {
	some res_name, res in input.resource.aws_launch_template
	some opts in as_list(res.metadata_options)
	opts.http_tokens != "required"
	msg := sprintf("aws_launch_template.%s: IMDSv2 http_tokens must be \"required\" (got %v)", [res_name, opts.http_tokens])
}

# --- Launch Template Block Device Encryption ---
deny contains msg if {
	some res_name, res in input.resource.aws_launch_template
	some bd in as_list(res.block_device_mappings)
	some ebs in as_list(bd.ebs)
	ebs.encrypted != true
	msg := sprintf("aws_launch_template.%s: root/data EBS block device mapping must set encrypted = true", [res_name])
}

# --- Literal Private Keys / Seed Defaults in Variables ---
secret_var_keywords := {"private_key", "mnemonic", "seed", "keystore", "secret_key", "session_key"}

deny contains msg if {
	some var_name, v in input.variable
	some kw in secret_var_keywords
	contains(lower(var_name), kw)
	def := object.get(v, "default", null)
	is_string(def)
	def != ""
	msg := sprintf("variable.%s: literal secret default is forbidden; load validator keys from Secrets Manager/KMS at runtime", [var_name])
}
