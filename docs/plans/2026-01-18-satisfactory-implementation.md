# Satisfactory Game Server Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Satisfactory as a deployable game server to the Bonfire platform.

**Architecture:** Create a thin game wrapper at `terraform/games/satisfactory/` that configures the existing `game-server` module with Satisfactory-specific settings (Docker image, ports, resource requirements).

**Tech Stack:** Terraform, AWS (EC2 Spot, S3), Docker (wolveix/satisfactory-server)

---

### Task 1: Create backend.tf

**Files:**
- Create: `terraform/games/satisfactory/backend.tf`

**Step 1: Create the directory**

Run: `mkdir -p terraform/games/satisfactory`

**Step 2: Create backend.tf with S3 state configuration**

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
    key     = "bonfire/satisfactory/terraform.tfstate"
    region  = "eu-north-1"
    encrypt = true
  }
}

provider "aws" {
  region = var.aws_region
}
```

**Step 3: Commit**

```bash
git add terraform/games/satisfactory/backend.tf
git commit -m "feat(satisfactory): add backend.tf with S3 state config"
```

---

### Task 2: Create variables.tf

**Files:**
- Create: `terraform/games/satisfactory/variables.tf`

**Step 1: Create variables.tf**

Note: Satisfactory doesn't need `world_name` or `server_pass` variables - these are configured in-game via the Server Manager UI.

```hcl
# AWS Configuration
variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

# Server Configuration
variable "max_players" {
  description = "Maximum number of players"
  type        = number
  default     = 4
}

# Instance Configuration
variable "instance_type" {
  description = "EC2 instance type (t3.large recommended for 8GB RAM)"
  type        = string
  default     = "t3.large"
}

variable "volume_size" {
  description = "Root volume size in GB"
  type        = number
  default     = 30
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "bonfire-satisfactory-key"
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

**Step 2: Commit**

```bash
git add terraform/games/satisfactory/variables.tf
git commit -m "feat(satisfactory): add variables.tf"
```

---

### Task 3: Create main.tf

**Files:**
- Create: `terraform/games/satisfactory/main.tf`

**Step 1: Create main.tf with game configuration and module calls**

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
    name         = "satisfactory"
    display_name = "Satisfactory Server"
    docker_image = "wolveix/satisfactory-server:latest"

    ports = {
      udp = [7777]
      tcp = []
    }

    env_vars = {
      MAXPLAYERS = tostring(var.max_players)
    }

    data_path    = "/config"
    backup_paths = ["/config/saved"]

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

**Step 2: Commit**

```bash
git add terraform/games/satisfactory/main.tf
git commit -m "feat(satisfactory): add main.tf with game config and modules"
```

---

### Task 4: Create outputs.tf

**Files:**
- Create: `terraform/games/satisfactory/outputs.tf`

**Step 1: Create outputs.tf**

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
  value       = var.enable_discord_bot ? module.discord_bot[0].api_endpoint : null
}
```

**Step 2: Commit**

```bash
git add terraform/games/satisfactory/outputs.tf
git commit -m "feat(satisfactory): add outputs.tf"
```

---

### Task 5: Create terraform.tfvars.example

**Files:**
- Create: `terraform/games/satisfactory/terraform.tfvars.example`

**Step 1: Create terraform.tfvars.example**

```hcl
# Satisfactory server configuration
# Note: Server name and passwords are configured in-game via Server Manager

# Optional: Player limit
# max_players = 4

# Optional: Instance configuration (t3.large recommended for 8GB RAM)
# instance_type = "t3.large"
# volume_size   = 30

# Optional: Discord bot (set enable_discord_bot = true to use)
# enable_discord_bot       = false
# discord_public_key       = ""
# discord_application_id   = ""
# discord_bot_token        = ""
# discord_authorized_users = []
```

**Step 2: Commit**

```bash
git add terraform/games/satisfactory/terraform.tfvars.example
git commit -m "feat(satisfactory): add terraform.tfvars.example"
```

---

### Task 6: Validate Terraform Configuration

**Step 1: Initialize Terraform**

Run: `cd terraform/games/satisfactory && terraform init`

Expected: Successful initialization with modules downloaded.

**Step 2: Validate configuration**

Run: `terraform validate`

Expected: `Success! The configuration is valid.`

**Step 3: Run plan (optional, requires AWS credentials)**

Run: `terraform plan`

Expected: Plan showing resources to create (S3 bucket, EC2 spot instance, security group, etc.)

**Step 4: Commit any fixes if validation revealed issues**

If changes were needed, commit them with appropriate message.

---

### Task 7: Final Commit and PR Prep

**Step 1: Verify all files are committed**

Run: `git status`

Expected: Clean working tree.

**Step 2: Review commit history**

Run: `git log --oneline main..HEAD`

Expected: Design doc + 5-6 implementation commits.

**Step 3: Push branch**

Run: `git push -u origin feat/satisfactory`

---

## Post-Implementation: First Deployment

After merging, deploy with:

```bash
cd terraform/games/satisfactory
cp terraform.tfvars.example terraform.tfvars  # Edit if needed
terraform init
terraform apply
```

Then in Satisfactory:
1. Open Server Manager
2. Add server using the public IP from terraform output
3. Claim the server (set Admin Password)
4. Set Player Password
5. Share Player Password with friends
