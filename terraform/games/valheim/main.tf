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
    name         = "valheim"
    display_name = var.server_name
    docker_image = "lloesche/valheim-server:latest"

    ports = {
      udp = [2456, 2457, 2458]
      tcp = []
    }

    env_vars = {
      SERVER_NAME   = var.server_name
      WORLD_NAME    = var.world_name
      SERVER_PASS   = var.server_pass
      SERVER_PUBLIC = "false"
      TZ            = var.timezone
      AUTO_UPDATE   = "1"
      AUTO_BACKUP   = "1"
    }

    # /config is where lloesche/valheim-server actually keeps world saves.
    # /opt/valheim is the game *install* directory — mounting /opt/valheim/data
    # there just grafts an unused subdirectory into the install tree, so world
    # data lived on the container's writable layer and was destroyed on every
    # `docker-compose down` (including the 30-minute idle auto-stop).
    #
    # Since Valheim 1.0 each world is a directory: worlds_local/<WorldName>/_main.0.fwl2
    data_path    = "/config"
    backup_paths = ["/config/worlds_local"]

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

