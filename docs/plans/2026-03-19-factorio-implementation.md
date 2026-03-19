# Factorio Game Server Implementation Plan

**Goal:** Add Factorio as a deployable game server to the Bonfire platform, following the same pattern as Valheim and Satisfactory.

**Architecture:** Create a thin game wrapper at `terraform/games/factorio/` that configures the existing `game-server` module with Factorio-specific settings. Requires a small extension to the `game-server` module to support an optional Docker Compose init service (for automated server password setup).

**Tech Stack:** Terraform, AWS (EC2 Spot, S3), Docker (`factoriotools/factorio:stable`)

---

## Spec: Factorio Configuration Decisions

### Docker Image

✅ **Confident:** `factoriotools/factorio:stable` — well-maintained community image, `stable` tag tracks the current stable Factorio release. Covers Factorio 2.0+ (Space Age DLC).

### Ports

✅ **Confident:**

| Port  | Protocol | Purpose      |
|-------|----------|--------------|
| 34197 | UDP      | Game traffic |

No TCP ports required. RCON not exposed (not needed for this use case).

### Instance Type

✅ **Confident:** `t3.medium` (2 vCPU / 4GB RAM). Comfortable for small friend groups (4–8 players). Factorio simulation is single-threaded, but t3.medium handles typical factory sizes without issue.

### Storage

✅ **Confident:** 30 GB EBS. Factorio saves are small (<100 MB typically); ample headroom for mods.

### Paths

✅ **Confident:** The `factoriotools/factorio` image uses `/factorio` as its data root.

| Purpose   | Container Path      |
|-----------|---------------------|
| Data root | `/factorio`         |
| Saves     | `/factorio/saves`   |
| Config    | `/factorio/config`  |
| Mods      | `/factorio/mods`    |

**Backup paths:** `["/factorio/saves", "/factorio/mods"]`

Config is intentionally excluded from backups — it is regenerated from defaults on first run, and restoring a stale config to a fresh instance is more likely to cause confusion than help. Mods are included so players don't need to manually re-sync after a restore.

### Environment Variables

⚠️ **Low confidence — needs verification before implementing:**

| Variable           | Value           | Purpose                          |
|--------------------|-----------------|----------------------------------|
| `SAVE_NAME`        | `var.save_name` | Which save file to load/create   |
| `GENERATE_NEW_SAVE`| `"true"`        | Create a new save if none exists |

<!-- TODO: Verify that SAVE_NAME and GENERATE_NEW_SAVE are actually supported env vars
     in factoriotools/factorio. Check the image entrypoint script on Docker Hub or
     https://github.com/factoriotools/factorio-docker. If not supported, remove from
     env_vars and document the alternative (e.g. save file is auto-created at default path). -->

### Server Password: Docker Compose Init Service

Rather than requiring manual SSH to set the server password, an init service in Docker Compose handles it automatically on first run.

**Design:**

1. Add `server_pass` as a sensitive Terraform variable (default `""` = no password).
2. When `server_pass` is non-empty, include an init service in docker-compose.yml that:
   - Runs using the same `factoriotools/factorio:stable` image (already pulled, no extra download)
   - Copies the bundled example config to `/factorio/config/server-settings.json` if it doesn't exist yet
   - Patches `game_password` in the JSON
   - Exits with code 0
3. The main game service declares `depends_on` the init service with `condition: service_completed_successfully`, so it only starts after the init completes.
4. `restart: "no"` on the init service ensures it doesn't re-run on container restarts.

**Why this is better than a polling script:**
- Dependency is declarative, no sleep/poll loops
- Init only runs when the config file is absent (idempotent: skip if file already exists)
- Same image = no extra pull, no extra tooling

**Sketch of the init service block (docker-compose):**

```yaml
  factorio-init:
    image: factoriotools/factorio:stable
    entrypoint: ["/bin/sh", "-c"]
    command: >
      if [ ! -f /factorio/config/server-settings.json ]; then
        mkdir -p /factorio/config &&
        cp /opt/factorio/data/server-settings.example.json
           /factorio/config/server-settings.json &&
        sed -i 's/"game_password": ""/"game_password": "${server_pass}"/'
            /factorio/config/server-settings.json;
      fi
    volumes:
      - /factorio:/factorio
    restart: "no"

  factorio-server:
    ...
    depends_on:
      factorio-init:
        condition: service_completed_successfully
```

<!-- TODO: Verify the path to the bundled example file inside the container.
     Expected: /opt/factorio/data/server-settings.example.json
     Check by running: docker run --rm factoriotools/factorio:stable find / -name "server-settings.example.json" 2>/dev/null -->

<!-- TODO: Verify the exact JSON key and default value for game_password in the example file.
     Expected: "game_password": ""
     The sed pattern above assumes this exact formatting; if indented differently or quoted
     differently it will silently fail to patch. Consider using python3 -c with json module
     instead of sed if the image has python3 available — more robust against formatting variance. -->

<!-- TODO: Verify that `condition: service_completed_successfully` is supported by the
     version of Docker Compose installed by user_data.sh.tpl.
     The template currently installs v2.20.0 which does support this condition, but confirm
     the docker-compose.yml version header doesn't need to change (Compose Spec vs v3 syntax). -->

**Required module changes:**

The `game-server` module's `docker-compose.yml.tpl` currently renders a single service. To support the init pattern, add an optional `init_service` field to the game config object. When non-empty, the template renders the init service block and adds `depends_on` to the main service.

<!-- TODO: Design the exact interface for init_service in the game config object and
     docker-compose.yml.tpl. Options:
     A) init_service as a structured map (image, command, volumes) — type-safe, more template work
     B) init_service_yaml as a raw YAML string injected verbatim — flexible, less safe
     Recommend option A for consistency with the rest of the game config schema.
     Also decide: should init_service only render when server_pass != ""? (conditional in main.tf)
     Or always render a no-op init service? The former is cleaner. -->

### Terraform Variables

| Variable               | Type    | Default                  | Required | Notes                                             |
|------------------------|---------|--------------------------|----------|---------------------------------------------------|
| `aws_region`           | string  | `"eu-north-1"`           | No       | AWS region                                        |
| `server_name`          | string  | `"Factorio Server"`      | No       | Informational label; actual server name set in server-settings.json |
| `server_pass`          | string  | `""`                     | No       | Join password; empty = no password. Sensitive.    |
| `save_name`            | string  | `"world"`                | No       | Save file name to create or load                  |
| `instance_type`        | string  | `"t3.medium"`            | No       | EC2 instance type                                 |
| `volume_size`          | number  | `30`                     | No       | EBS volume size in GB                             |
| `ssh_key_name`         | string  | `"bonfire-factorio-key"` | No       | Key pair name                                     |
| `public_key`           | string  | `""`                     | No       | BYO public key; auto-generated if empty           |
| `enable_eip`           | bool    | `true`                   | No       | Allocate Elastic IP                               |
| `backup_retention_days`| number  | `7`                      | No       | Days to retain old S3 backup versions             |
| `enable_discord_bot`   | bool    | `false`                  | No       | Deploy Discord bot Lambda                         |
| Discord vars           | various | (same as other games)    | No       | Standard Discord bot config                       |

> **`max_players`:** Not exposed as a Terraform variable. The `factoriotools/factorio` image does not appear to support it via env var; it is set in `server-settings.json`. Since we're already patching that file via the init service, it could be added to the init script later if desired.
>
> <!-- TODO: Confirm whether max_players (or similar) can be set via env var in this image.
>      If yes, add it as a variable alongside server_pass. -->

### State Backend

✅ **Confident:** S3 key `bonfire/factorio/terraform.tfstate` in the existing `valheim-ec2-tf-state` bucket.

---

## Implementation Tasks

### Task 0: Extend game-server module — init service support

**Files to modify:**
- `terraform/modules/game-server/templates/docker-compose.yml.tpl`
- `terraform/modules/game-server/ec2.tf` (locals that build the game config)
- `terraform/modules/game-server/variables.tf` (if game object schema needs updating)

<!-- TODO: Work out the full module changes before implementing the Factorio game files.
     The init_service interface needs to be settled first so main.tf can reference it correctly.
     See TODO in "Server Password: Docker Compose Init Service" section above. -->

**What changes:**
- `docker-compose.yml.tpl` gains an optional init service block and conditional `depends_on` on the main service
- The game config object gains an optional `init_service` field (or equivalent)
- Existing games (Valheim, Satisfactory) are unaffected — they leave `init_service` empty/null

**Commit:** `feat(game-server): add optional init service support to docker-compose template`

---

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

variable "server_pass" {
  description = "Server join password. Empty string = no password (public server)."
  type        = string
  sensitive   = true
  default     = ""
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

> ⚠️ The `init_service` field shape below is a placeholder — finalize after Task 0 settles the interface.

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
      # TODO: Verify SAVE_NAME and GENERATE_NEW_SAVE are supported by the image before shipping
      SAVE_NAME         = var.save_name
      GENERATE_NEW_SAVE = "true"
    }

    data_path    = "/factorio"
    backup_paths = ["/factorio/saves", "/factorio/mods"]

    # Init service patches server-settings.json with the password on first run.
    # Only included when server_pass is set; empty string skips the init service entirely.
    # TODO: Replace init_service with the actual field name/shape decided in Task 0.
    init_service = var.server_pass != "" ? {
      image   = "factoriotools/factorio:stable"
      command = <<-EOT
        if [ ! -f /factorio/config/server-settings.json ]; then
          mkdir -p /factorio/config &&
          cp /opt/factorio/data/server-settings.example.json /factorio/config/server-settings.json &&
          sed -i 's/"game_password": ""/"game_password": "${var.server_pass}"/' /factorio/config/server-settings.json;
        fi
      EOT
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

# Required: set a join password (leave empty for a public server)
# server_pass = "your-password-here"

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

### Task 7: Push

```bash
git push -u origin claude/plan-factorio-support-dBYrQ
```

---

## Post-Deployment: First-Time Setup

```bash
cd terraform/games/factorio
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars: set server_pass
terraform init
terraform apply
```

The init service runs automatically on first start and sets the password. No SSH required.

To connect:
1. Get the server IP: `terraform output public_ip`
2. Friends connect via **Multiplayer → Connect to address** in the Factorio client using `<ip>:34197`
3. Enter the password when prompted

> **DLC note:** The server does not need to own Space Age. Each player's client provides DLC access — the server just runs the game engine.

> <!-- TODO: Verify the DLC note above. This is the expected behaviour based on how Factorio
>      has handled DLC historically, but confirm with the factoriotools image docs or
>      Factorio's dedicated server documentation for 2.0+. -->
