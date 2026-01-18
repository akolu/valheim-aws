# Bonfire: Multi-Game Server Platform

## Overview

Refactor the Valheim-specific AWS infrastructure into a generic game server hosting platform called **Bonfire**. The platform should support multiple games with minimal configuration per game while keeping infrastructure code DRY.

## Goals

- Support N games with a single set of infrastructure modules
- Adding a new game requires only a thin configuration wrapper
- Each game is deployed independently with its own Terraform state
- One Discord bot per game (with path to future multi-game bot)

## Architecture

### Directory Structure

```
terraform/
  modules/
    game-server/       # Generic EC2 spot instance module (renamed from ec2-spot)
    discord-bot/       # Generic bot module (minor updates)
    s3-backup/         # Generic backup module (unchanged)
  games/
    valheim/
      main.tf          # locals block + module wiring
      variables.tf     # User inputs (passwords, instance type, etc.)
      backend.tf       # S3 state: key = "bonfire/valheim/terraform.tfstate"
      outputs.tf
    satisfactory/
      (same structure)

discord_bot/
  src/index.js         # Update command names to /server <game> <action>
  register-commands.js # Update command registration
```

### Game Configuration

Each game is defined via a `locals` block containing all game-specific settings:

```hcl
# terraform/games/valheim/main.tf

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
      UPDATE_CRON   = "*/15 * * * *"
      BACKUPS       = "true"
      TZ            = var.timezone
    }

    data_path    = "/opt/valheim/data"
    backup_paths = ["/opt/valheim/data/worlds_local"]

    resources = {
      instance_type = "t3.medium"
      volume_size   = 30
    }
  }
}
```

Example for Satisfactory:

```hcl
# terraform/games/satisfactory/main.tf

locals {
  # ... base_tags, game_tags, tags same pattern ...

  game = {
    name         = "satisfactory"
    display_name = var.server_name
    docker_image = "wolveix/satisfactory-server:latest"

    ports = {
      udp = [7777, 15000, 15777]
      tcp = []
    }

    env_vars = {
      MAXPLAYERS     = var.max_players
      SERVERGAMEPORT = "7777"
    }

    data_path    = "/opt/satisfactory/data"
    backup_paths = ["/opt/satisfactory/data/saved"]

    resources = {
      instance_type = "t3.large"
      volume_size   = 50
    }
  }
}
```

### Module Interface

The game-server module accepts a `game` object:

```hcl
# terraform/modules/game-server/variables.tf

variable "game" {
  description = "Game configuration object"
  type = object({
    name         = string
    display_name = string
    docker_image = string
    ports = object({
      udp = list(number)
      tcp = list(number)
    })
    env_vars     = map(string)
    data_path    = string
    backup_paths = list(string)
    resources = optional(object({
      instance_type = optional(string, "t3.medium")
      volume_size   = optional(number, 30)
    }), {})
  })
}
```

### Generic Templates

The docker-compose template becomes fully generic:

```yaml
version: "3"
services:
  game-server:
    image: ${docker_image}
    container_name: ${game_name}-server
    restart: unless-stopped
    ports:
%{ for port in udp_ports ~}
      - "${port}:${port}/udp"
%{ endfor ~}
%{ for port in tcp_ports ~}
      - "${port}:${port}/tcp"
%{ endfor ~}
    environment:
%{ for key, value in env_vars ~}
      - ${key}=${value}
%{ endfor ~}
    volumes:
      - ${data_path}:${data_path}
```

### Tagging Strategy

All resources tagged with:
- `Project = bonfire` (identifies resources from this platform)
- `Game = <game-name>` (distinguishes which game)
- `ManagedBy = terraform`

### Discord Bot

**Command structure:** `/server <game> <action>`

Examples:
- `/server valheim status`
- `/server valheim start`
- `/server satisfactory stop`

This uses Discord's subcommand groups:
```
/server (command)
  └── valheim (subcommand group)
        ├── status
        ├── start
        └── stop
  └── satisfactory (subcommand group)
        ├── status
        ├── start
        └── stop
```

**Current scope:** One bot per game, minimal changes to existing code.

**Future option:** Single bot controlling multiple games (command structure already supports this).

## Migration Plan

1. **Create new directory structure**
   - Set up `terraform/modules/game-server/` (genericized from ec2-spot)
   - Update `terraform/modules/discord-bot/` (add game_name, update commands)
   - Keep `terraform/modules/s3-backup/` as-is
   - Create `terraform/games/valheim/` wrapper

2. **Migrate Terraform state**
   - Move state to new key: `bonfire/valheim/terraform.tfstate`

3. **Verify Valheim works**
   - `terraform apply` in new structure
   - Confirm EC2 instance launches
   - Confirm Valheim server is accessible (connect with game client)
   - Confirm Discord bot commands work
   - Confirm backups still function

4. **Update Discord bot commands**
   - Change to `/server valheim <action>` structure
   - Re-register with Discord API
   - Test commands again

5. **Clean up**
   - Delete old `terraform/valheim-server/` directory

6. **Add Satisfactory**
   - Copy `terraform/games/valheim/`, update `locals` block
   - Deploy and verify

## Deferred Work

- **Discord bot refactor:** Current code works but is poorly structured. Defer full rewrite until multi-game-single-bot feature is needed.

## Decisions Made

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Project name | Bonfire | Memorable, gaming-relevant (Dark Souls checkpoint), minimal naming collisions |
| Directory structure | Hybrid (shared modules + game directories) | Code reuse via modules, clear separation per game |
| Game config format | locals block | All game config visible in one place |
| Discord commands | /server \<game\> \<action\> | Clean structure, future-proof for multi-game bot |
| Resource config | Optional per-game with defaults | Different games need different instance sizes |
| Bot per game vs single bot | One per game (for now) | Simpler, defers complexity |
