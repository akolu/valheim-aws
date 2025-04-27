# Main configuration file for Valheim server on AWS EC2 with Spot pricing

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project   = "Valheim-Server"
      Terraform = "true"
    }
  }
}

# Simple S3 bucket for backups with deterministic name
resource "aws_s3_bucket" "backups" {
  bucket = "${var.instance_name}-backups-${var.aws_region}"

  tags = {
    Name = "${var.instance_name}-backup-bucket"
  }
}

# EC2 Spot instance for Valheim server
module "valheim_server" {
  source = "../modules/ec2-spot"

  instance_name       = var.instance_name
  valheim_world_name  = var.valheim_world_name
  valheim_server_name = var.valheim_server_name
  valheim_server_pass = var.valheim_server_pass
  aws_region          = var.aws_region
  instance_type       = var.instance_type
  backup_s3_bucket    = aws_s3_bucket.backups.bucket

  # If you have your own SSH key pair
  ssh_key_name = var.ssh_key_name
  public_key   = var.public_key
}

# Optional Discord bot module - comment out if not needed
module "discord_bot" {
  source = "../modules/discord-bot"
  count  = var.enable_discord_bot ? 1 : 0

  prefix      = var.instance_name
  instance_id = module.valheim_server.instance_id
  aws_region  = var.aws_region

  # Discord configuration
  discord_bot_zip_path     = var.discord_bot_zip_path
  discord_public_key       = var.discord_public_key
  discord_application_id   = var.discord_application_id
  discord_bot_token        = var.discord_bot_token
  discord_authorized_users = var.discord_authorized_users
  discord_authorized_roles = var.discord_authorized_roles

  # Command registration (optional)
  discord_bot_dir = var.discord_bot_dir
}
