# Default VPC data source
data "aws_vpc" "default" {
  default = true
}

# Security group for game server
resource "aws_security_group" "game_server_sg" {
  name        = "${local.game_name}-server-sg"
  description = "Security group for ${local.display_name}"
  vpc_id      = data.aws_vpc.default.id

  # SSH access
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = var.allowed_ssh_cidr_blocks
    description = "SSH access"
  }

  # UDP game ports
  dynamic "ingress" {
    for_each = local.udp_ports
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "udp"
      cidr_blocks = ["0.0.0.0/0"]
      description = "${local.display_name} UDP port ${ingress.value}"
    }
  }

  # TCP game ports
  dynamic "ingress" {
    for_each = local.tcp_ports
    content {
      from_port   = ingress.value
      to_port     = ingress.value
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
      description = "${local.display_name} TCP port ${ingress.value}"
    }
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound traffic"
  }

  tags = merge(var.tags, {
    Name = "${local.game_name}-server-sg"
  })
}
