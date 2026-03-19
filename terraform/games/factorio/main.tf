locals {
  base_tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
  }

  game_tags = {
    Game = local.game.name
  }

  tags = merge(local.base_tags, local.game_tags)

  game = {
    name         = "factorio"
    display_name = var.server_name
    docker_image = "factoriotools/factorio:stable"

    ports = {
      udp = [34197]
      tcp = []
    }

    env_vars = {
      SAVE_NAME         = var.save_name
      GENERATE_NEW_SAVE = "true"
      DLC_SPACE_AGE     = var.dlc_space_age
    }

    data_path    = "/factorio"
    backup_paths = ["/factorio/saves", "/factorio/mods"]

    # Init service patches server-settings.json with the join password.
    # Password is passed as an env var, never interpolated into the command string.
    # $$ escaping prevents docker-compose from interpreting shell variables at parse time.
    init_service = var.server_pass != "" ? {
      image   = "factoriotools/factorio:stable"
      command = chomp(<<-EOT
        if [ ! -f /factorio/config/server-settings.json ]; then
        mkdir -p /factorio/config &&
        cp /opt/factorio/data/server-settings.example.json /factorio/config/server-settings.json;
        fi &&
        tmp=$$(mktemp) &&
        jq --arg pw "$$SERVER_PASS" '.game_password = $$pw' /factorio/config/server-settings.json > "$$tmp" &&
        mv "$$tmp" /factorio/config/server-settings.json
        EOT
      )
      env_vars = { SERVER_PASS = sensitive(var.server_pass) }
    } : null

    resources = {
      instance_type = var.instance_type
      volume_size   = var.volume_size
    }
  }
}

module "backups" {
  source = "../../modules/s3-backup"

  bucket_name                       = "bonfire-${local.game.name}-backups-${var.aws_region}"
  noncurrent_version_retention_days = var.backup_retention_days

  tags = merge(local.tags, {
    Name = "bonfire-${local.game.name}-backups"
  })
}

module "game_server" {
  source = "../../modules/game-server"

  game             = local.game
  aws_region       = var.aws_region
  backup_s3_bucket = module.backups.bucket_name
  ssh_key_name     = var.ssh_key_name
  public_key       = var.public_key
  enable_eip       = var.enable_eip
  tags             = local.tags
}

module "discord_bot" {
  source = "../../modules/discord-bot"
  count  = var.enable_discord_bot ? 1 : 0

  game_name    = local.game.name
  prefix       = "bonfire-${local.game.name}"
  instance_id  = module.game_server.instance_id
  aws_region   = var.aws_region

  discord_bot_zip_path     = var.discord_bot_zip_path
  discord_public_key       = var.discord_public_key
  discord_application_id   = var.discord_application_id
  discord_bot_token        = var.discord_bot_token
  discord_authorized_users = var.discord_authorized_users
  discord_authorized_roles = var.discord_authorized_roles
  discord_bot_dir          = var.discord_bot_dir

  tags = local.tags
}
