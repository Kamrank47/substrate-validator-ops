terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

locals {
  name_prefix = "validator-${var.network}-${var.operator_role}"
  common_tags = merge(var.tags, {
    Project   = "substrate-validator-ops"
    Network   = var.network
    Operator  = var.operator_role
    ManagedBy = "OpenTofu"
    SLOTarget = "99.99%"
  })
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-vpc"
  })
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-igw"
  })
}

resource "aws_subnet" "sentry_public" {
  count = length(var.availability_zones)

  vpc_id                  = aws_vpc.this.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index)
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-sentry-pub-${count.index}"
    Tier = "sentry-nlb"
  })
}

resource "aws_subnet" "validator_private" {
  count = length(var.availability_zones)

  vpc_id                  = aws_vpc.this.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index + 8)
  availability_zone       = var.availability_zones[count.index]
  map_public_ip_on_launch = false

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-validator-priv-${count.index}"
    Tier = "validator-private"
  })
}

resource "aws_route_table" "sentry_public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-pub-rt"
  })
}

resource "aws_route_table_association" "sentry_public" {
  count = length(var.availability_zones)

  subnet_id      = aws_subnet.sentry_public[count.index].id
  route_table_id = aws_route_table.sentry_public.id
}

# Only libp2p port 30333 is open to 0.0.0.0/0. RPC (9944) and Prometheus (9615) stay internal.
resource "aws_security_group" "validator" {
  name        = "${local.name_prefix}-sg"
  description = "Substrate validator: P2P 30333 public via NLB; RPC 9944 and metrics 9615 restricted"
  vpc_id      = aws_vpc.this.id

  ingress {
    description = "Substrate libp2p validator gossip (P2P)"
    from_port   = 30333
    to_port     = 30333
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description = "Validator JSON-RPC / WebSocket restricted to ops VPN CIDR"
    from_port   = 9944
    to_port     = 9944
    protocol    = "tcp"
    cidr_blocks = var.ops_cidr_blocks
  }

  ingress {
    description = "Substrate Prometheus exporter restricted to monitoring CIDR"
    from_port   = 9615
    to_port     = 9615
    protocol    = "tcp"
    cidr_blocks = var.monitoring_cidr_blocks
  }

  egress {
    description = "Outbound peer discovery, snapshots, and telemetry"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-sg"
  })
}

resource "aws_lb" "p2p" {
  name               = substr("${local.name_prefix}-nlb", 0, 32)
  internal           = false
  load_balancer_type = "network"
  subnets            = aws_subnet.sentry_public[*].id

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-nlb"
  })
}

resource "aws_lb_target_group" "p2p" {
  name        = substr("${local.name_prefix}-p2p", 0, 32)
  port        = 30333
  protocol    = "TCP"
  vpc_id      = aws_vpc.this.id
  target_type = "instance"

  health_check {
    protocol            = "TCP"
    port                = "30333"
    healthy_threshold   = 2
    unhealthy_threshold = 2
    interval            = 10
  }

  tags = local.common_tags
}

resource "aws_lb_listener" "p2p" {
  load_balancer_arn = aws_lb.p2p.arn
  port              = 30333
  protocol          = "TCP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.p2p.arn
  }
}

resource "aws_kms_key" "chain_state" {
  description             = "KMS key for ${local.name_prefix} chain database and session keystore"
  deletion_window_in_days = 14
  enable_key_rotation     = true

  tags = local.common_tags
}

resource "aws_ebs_volume" "chain_state" {
  availability_zone = var.availability_zones[0]
  size              = var.chain_volume_gb
  type              = "gp3"
  iops              = 6000
  throughput        = 400
  encrypted         = true
  kms_key_id        = aws_kms_key.chain_state.arn

  tags = merge(local.common_tags, {
    Name = "${local.name_prefix}-chain-state"
  })
}

resource "aws_secretsmanager_secret" "validator_keystore" {
  name                    = "${local.name_prefix}-session-keystore"
  description             = "Reference secret storing encrypted Substrate Aura/Grandpa session keys (populated out-of-band)"
  kms_key_id              = aws_kms_key.chain_state.arn
  recovery_window_in_days = 14

  tags = local.common_tags
}

resource "aws_iam_role" "validator" {
  name = "${local.name_prefix}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "validator_keystore_read" {
  name = "${local.name_prefix}-keystore-read"
  role = aws_iam_role.validator.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "secretsmanager:GetSecretValue",
          "secretsmanager:DescribeSecret"
        ]
        Resource = aws_secretsmanager_secret.validator_keystore.arn
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Decrypt"
        ]
        Resource = aws_kms_key.chain_state.arn
      }
    ]
  })
}

resource "aws_iam_instance_profile" "validator" {
  name = "${local.name_prefix}-profile"
  role = aws_iam_role.validator.name
}

resource "aws_launch_template" "validator" {
  name_prefix   = "${local.name_prefix}-lt-"
  image_id      = var.ami_id
  instance_type = var.instance_type

  iam_instance_profile {
    name = aws_iam_instance_profile.validator.name
  }

  vpc_security_group_ids = [aws_security_group.validator.id]

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 1
  }

  block_device_mappings {
    device_name = "/dev/sda1"

    ebs {
      volume_size           = 60
      volume_type           = "gp3"
      encrypted             = true
      kms_key_id            = aws_kms_key.chain_state.arn
      delete_on_termination = true
    }
  }

  user_data = base64encode(<<-EOF
    #!/usr/bin/env bash
    set -euo pipefail
    echo "CHAIN_NETWORK=${var.network}" >> /etc/environment
    echo "OPERATOR_ROLE=${var.operator_role}" >> /etc/environment
    echo "KEYSTORE_SECRET_ARN=${aws_secretsmanager_secret.validator_keystore.arn}" >> /etc/environment
  EOF
  )

  tag_specifications {
    resource_type = "instance"
    tags = merge(local.common_tags, {
      Name = "${local.name_prefix}-node"
    })
  }
}

# Strict cardinality 1 prevents accidental active/active equivocation (double-signing).
resource "aws_autoscaling_group" "validator" {
  name                = "${local.name_prefix}-asg"
  min_size            = 1
  max_size            = 1
  desired_capacity    = 1
  vpc_zone_identifier = [aws_subnet.validator_private[0].id]
  target_group_arns   = [aws_lb_target_group.p2p.arn]

  launch_template {
    id      = aws_launch_template.validator.id
    version = "$Latest"
  }

  tag {
    key                 = "Name"
    value               = "${local.name_prefix}-node"
    propagate_at_launch = true
  }
}
