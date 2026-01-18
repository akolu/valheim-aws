# Use data source for latest Amazon Linux 2023 AMI
data "aws_ami" "amazon_linux" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-kernel-*-x86_64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# Local variables derived from the game configuration object
locals {
  game_name    = var.game.name
  display_name = var.game.display_name
  docker_image = var.game.docker_image
  udp_ports    = var.game.ports.udp
  tcp_ports    = var.game.ports.tcp
  env_vars     = var.game.env_vars
  data_path    = var.game.data_path
  backup_paths = var.game.backup_paths

  instance_type = try(var.game.resources.instance_type, "t3.medium")
  volume_size   = try(var.game.resources.volume_size, 30)

  instance_name = "${var.game.name}-server"
}

# Generate template content
locals {
  docker_compose_content = templatefile("${path.module}/templates/docker-compose.yml.tpl", {
    game_name    = local.game_name
    docker_image = local.docker_image
    udp_ports    = local.udp_ports
    tcp_ports    = local.tcp_ports
    env_vars     = local.env_vars
    data_path    = local.data_path
  })

  backup_script_content = templatefile("${path.module}/templates/backup.sh.tpl", {
    game_name    = local.game_name
    s3_bucket    = var.backup_s3_bucket
    backup_paths = local.backup_paths
  })

  restore_script_content = templatefile("${path.module}/templates/restore.sh.tpl", {
    game_name = local.game_name
    s3_bucket = var.backup_s3_bucket
    data_path = local.data_path
  })

  user_data = templatefile("${path.module}/templates/user_data.sh.tpl", {
    game_name              = local.game_name
    display_name           = local.display_name
    data_path              = local.data_path
    docker_compose_content = local.docker_compose_content
    backup_script_content  = local.backup_script_content
    restore_script_content = local.restore_script_content
  })
}

# Request a spot instance
resource "aws_spot_instance_request" "game_server" {
  ami                    = var.ami_id != "" ? var.ami_id : data.aws_ami.amazon_linux.id
  instance_type          = local.instance_type
  key_name               = aws_key_pair.game_server_key.key_name
  vpc_security_group_ids = [aws_security_group.game_server_sg.id]
  iam_instance_profile   = aws_iam_instance_profile.game_server_profile.name

  spot_type            = "persistent"
  wait_for_fulfillment = true

  user_data = local.user_data

  root_block_device {
    volume_size = local.volume_size
    volume_type = "gp3"
  }

  tags = merge(var.tags, {
    Name = local.instance_name
  })

  # Ensure instance is not terminated when spot request is cancelled
  instance_interruption_behavior = "stop"
}
