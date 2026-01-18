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

# Generate docker-compose content
locals {
  docker_compose = templatefile("${path.module}/templates/docker-compose.yml.tpl", {
    server_name = var.valheim_server_name
    world_name  = var.valheim_world_name
    server_pass = var.valheim_server_pass
  })

  backup_script = templatefile("${path.module}/templates/backup.sh.tpl", {
    world_name = var.valheim_world_name
    s3_bucket  = var.backup_s3_bucket
  })

  restore_script = templatefile("${path.module}/templates/restore.sh.tpl", {
    world_name = var.valheim_world_name
    s3_bucket  = var.backup_s3_bucket
  })
}

# Request a spot instance
resource "aws_spot_instance_request" "valheim_server" {
  ami                    = var.ami_id != "" ? var.ami_id : data.aws_ami.amazon_linux.id
  instance_type          = var.instance_type
  key_name               = aws_key_pair.valheim_key.key_name
  vpc_security_group_ids = [aws_security_group.valheim_sg.id]
  iam_instance_profile   = aws_iam_instance_profile.cloudwatch_profile.name

  spot_type            = "persistent"
  wait_for_fulfillment = true

  user_data = templatefile("${path.module}/templates/user_data.sh.tpl", {
    docker_compose_content = local.docker_compose
    backup_script_content  = local.backup_script
    restore_script_content = local.restore_script
    s3_bucket              = var.backup_s3_bucket
    world_name             = var.valheim_world_name
  })

  tags = {
    Name = var.instance_name
  }

  # Ensure instance is not terminated when spot request is cancelled
  instance_interruption_behavior = "stop"
}
