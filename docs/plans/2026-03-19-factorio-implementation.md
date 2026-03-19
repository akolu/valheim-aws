# Factorio Game Server Implementation Plan

**Goal:** Add Factorio as a deployable game server to the Bonfire platform, following the same pattern as Valheim and Satisfactory.

**Architecture:** Create a thin game wrapper at `terraform/games/factorio/` that configures the existing `game-server` module with Factorio-specific settings.

**Tech Stack:** Terraform, AWS (EC2 Spot, S3), Docker (`factoriotools/factorio:stable`)

---

## Spec: Factorio Configuration Decisions

### Docker Image

`factoriotools/factorio:stable` — The `stable` tag tracks the current stable release, which is sufficient for Factorio 2.0+ (Space Age DLC). No need for `latest`; both are equivalent in practice for this image.

### Ports

| Port  | Protocol | Purpose        |
|-------|----------|----------------|
| 34197 | UDP      | Game traffic   |

No TCP ports required. Factorio's RCON is not exposed (unnecessary for this use case).

### Instance Type

`t3.medium` (2 vCPU / 4GB RAM). Comfortable for small groups (4–8 players). Factorio is single-threaded for UPS-heavy maps, but t3.medium handles typical friend-group factory sizes well.

### Storage

30 GB GP3 EBS. Factorio saves are small (typically <100 MB), so this is ample even with mods.

### Paths

| Purpose     | Container Path       |
|-------------|----------------------|
| Data root   | `/factorio`          |
| Backups     | `/factorio/saves`, `/factorio/mods` |

Config is excluded from backups (regenerated on first run with defaults). Mods are included so friends don't need to manually re-sync after a restore.

### Environment Variables

The `factoriotools/factorio` image primarily uses `server-settings.json` for server configuration (name, password, max players). The image does support a small set of env vars for bootstrap behavior:

| Variable           | Value              | Purpose                              |
|--------------------|--------------------|--------------------------------------|
| `SAVE_NAME`        | `var.save_name`    | Which save file to load/create       |
| `GENERATE_NEW_SAVE`| `"true"`           | Create a new save if none exists     |

Server name, join password, and max players are configured post-deployment by editing `/factorio/config/server-settings.json` (created with defaults on first run). SSH access is provided for this via the standard bonfire SSH workflow.

### Terraform Variables

| Variable               | Type    | Default                | Required | Notes                                      |
|------------------------|---------|------------------------|----------|--------------------------------------------|
| `aws_region`           | string  | `"eu-north-1"`         | No       | AWS region                                 |
| `server_name`          | string  | `"Factorio Server"`    | No       | Label used in Terraform (informational)    |
| `save_name`            | string  | `"world"`              | No       | Name of save file to create/load           |
| `instance_type`        | string  | `"t3.medium"`          | No       | EC2 instance type                          |
| `volume_size`          | number  | `30`                   | No       | EBS volume size in GB                      |
| `ssh_key_name`         | string  | `"bonfire-factorio-key"` | No     | Key pair name                              |
| `public_key`           | string  | `""`                   | No       | BYO public key; auto-generated if empty    |
| `enable_eip`           | bool    | `true`                 | No       | Allocate Elastic IP                        |
| `backup_retention_days`| number  | `7`                    | No       | Days to retain old backup versions in S3   |
| `enable_discord_bot`   | bool    | `false`                | No       | Deploy Discord bot Lambda                  |
| Discord vars           | various | (same as other games)  | No       | Standard Discord bot config                |

> **Note:** No `server_pass` or `max_players` Terraform variables — these are set in `server-settings.json` after first deployment, consistent with Satisfactory's approach (configure in-game/via config rather than through env vars).

### State Backend

S3 key: `bonfire/factorio/terraform.tfstate` (same bucket as other games: `valheim-ec2-tf-state`).

---

## Implementation Tasks

### Task 1: Create `backend.tf`

**File:** `terraform/games/factorio/backend.tf`

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }

  backend "s3" {
    bucket  = "valheim-ec2-tf-state"
    key     = "bonfire/factorio/terraform.tfstate"
    region  = "eu-north-1"
    encrypt = true
  }
}

provider "aws" {
  region = var.aws_region
}
```

**Commit:** `feat(factorio): add backend.tf with S3 state config`

---

### Task 2: Create `variables.tf`

**File:** `terraform/games/factorio/variables.tf`

```hcl
# AWS Configuration
variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

# Server Configuration
variable "server_name" {
  description = "Display name label for the server (informational; actual name set in server-settings.json)"
  type        = string
  default     = "Factorio Server"
}

variable "save_name" {
  description = "Name of the Factorio save file to create or load"
  type        = string
  default     = "world"
}

# Instance Configuration
variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.medium"
}

variable "volume_size" {
  description = "Root volume size in GB"
  type        = number
  default     = 30
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "bonfire-factorio-key"
}

variable "public_key" {
  description = "Public key material (optional, will generate if empty)"
  type        = string
  default     = ""
}

variable "enable_eip" {
  description = "Whether to allocate an Elastic IP"
  type        = bool
  default     = true
}

# Backup Configuration
variable "backup_retention_days" {
  description = "Number of days to retain old backup versions"
  type        = number
  default     = 7
}

# Discord Bot Configuration
variable "enable_discord_bot" {
  description = "Whether to deploy the Discord bot"
  type        = bool
  default     = false
}

variable "discord_public_key" {
  description = "Discord application public key"
  type        = string
  default     = ""
}

variable "discord_application_id" {
  description = "Discord application ID"
  type        = string
  default     = ""
}

variable "discord_bot_token" {
  description = "Discord bot token"
  type        = string
  sensitive   = true
  default     = ""
}

variable "discord_authorized_users" {
  description = "Discord user IDs authorized to control the server"
  type        = list(string)
  default     = []
}

variable "discord_authorized_roles" {
  description = "Discord role names authorized to control the server"
  type        = list(string)
  default     = ["Admin"]
}

variable "discord_bot_zip_path" {
  description = "Path to Discord bot Lambda zip file"
  type        = string
  default     = "../../../discord_bot/bonfire_discord_bot.zip"
}

variable "discord_bot_dir" {
  description = "Path to Discord bot source directory"
  type        = string
  default     = "../../../discord_bot"
}
```

**Commit:** `feat(factorio): add variables.tf`

---

### Task 3: Create `main.tf`

**File:** `terraform/games/factorio/main.tf`

```hcl
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
    }

    data_path    = "/factorio"
    backup_paths = ["/factorio/saves", "/factorio/mods"]

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
```

**Commit:** `feat(factorio): add main.tf with game config and modules`

---

### Task 4: Create `outputs.tf`

**File:** `terraform/games/factorio/outputs.tf`

```hcl
output "instance_id" {
  description = "EC2 instance ID"
  value       = module.game_server.instance_id
}

output "public_ip" {
  description = "Public IP address of the server"
  value       = module.game_server.public_ip
}

output "ssh_command" {
  description = "SSH command to connect to the server"
  value       = module.game_server.ssh_command
}

output "discord_bot_endpoint" {
  description = "Discord bot API endpoint"
  value       = var.enable_discord_bot ? module.discord_bot[0].discord_bot_url : null
}
```

**Commit:** `feat(factorio): add outputs.tf`

---

### Task 5: Create `terraform.tfvars.example`

**File:** `terraform/games/factorio/terraform.tfvars.example`

```hcl
# Factorio server configuration
#
# After first deploy, SSH in and edit server settings:
#   ssh -i <key>.pem ec2-user@<public_ip>
#   sudo nano /opt/factorio/data/config/server-settings.json
# Set: name, description, game_password, max_players, visibility

# Optional: save file name (created fresh on first run)
# save_name = "world"

# Optional: instance configuration
# instance_type = "t3.medium"
# volume_size   = 30

# Optional: Discord bot (set enable_discord_bot = true to use)
# enable_discord_bot       = false
# discord_public_key       = ""
# discord_application_id   = ""
# discord_bot_token        = ""
# discord_authorized_users = []
```

**Commit:** `feat(factorio): add terraform.tfvars.example`

---

### Task 6: Validate Terraform Configuration

```bash
cd terraform/games/factorio
terraform init
terraform validate
```

Expected: `Success! The configuration is valid.`

If issues found, fix and commit with `fix(factorio): ...` message.

---

### Task 7: Commit Plan Doc and Push

```bash
git add docs/plans/2026-03-19-factorio-implementation.md
git commit -m "docs: add Factorio implementation plan"
git push -u origin claude/plan-factorio-support-dBYrQ
```

---

## Post-Deployment: First-Time Setup

```bash
cd terraform/games/factorio
cp terraform.tfvars.example terraform.tfvars  # edit if desired
terraform init
terraform apply
```

Then configure the server:

1. SSH in: `ssh -i bonfire-factorio-key.pem ec2-user@<public_ip>`
2. Edit server settings:
   ```bash
   sudo nano /opt/factorio/data/config/server-settings.json
   ```
3. Set `name`, `game_password`, `max_players`, and `visibility` fields
4. Restart the service: `sudo systemctl restart factorio`
5. Share the public IP and game password with friends
6. Friends connect via **Multiplayer → Connect to address** in the Factorio client

> **DLC note:** The server does not need to own Space Age. Each player's client provides DLC access. The server just runs the game; clients unlock the content.
