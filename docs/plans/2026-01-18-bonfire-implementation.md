# Bonfire Multi-Game Platform Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Refactor Valheim-specific infrastructure into a generic game server hosting platform called Bonfire.

**Architecture:** Shared modules in `terraform/modules/` accept a `game` configuration object. Thin game wrappers in `terraform/games/<game>/` define game-specific settings via locals blocks. Discord bot uses `/server <game> <action>` command structure.

**Tech Stack:** Terraform, AWS (EC2 Spot, S3, Lambda, API Gateway), Docker, Node.js (Discord bot)

---

## Task 1: Create Directory Structure

**Files:**
- Create: `terraform/modules/game-server/` (directory)
- Create: `terraform/games/valheim/` (directory)

**Step 1: Create new directory structure**

```bash
mkdir -p terraform/modules/game-server/templates
mkdir -p terraform/games/valheim
```

**Step 2: Copy ec2-spot module to game-server**

```bash
cp terraform/modules/ec2-spot/*.tf terraform/modules/game-server/
cp terraform/modules/ec2-spot/templates/*.tpl terraform/modules/game-server/templates/
```

**Step 3: Verify files copied**

```bash
ls terraform/modules/game-server/
ls terraform/modules/game-server/templates/
```

Expected: All .tf files and .tpl templates present

**Step 4: Commit**

```bash
git add terraform/modules/game-server/
git commit -m "chore: copy ec2-spot module to game-server as starting point"
```

---

## Task 2: Genericize game-server Module Variables

**Files:**
- Modify: `terraform/modules/game-server/variables.tf`

**Step 1: Replace Valheim-specific variables with generic game object**

Replace the entire content of `terraform/modules/game-server/variables.tf` with:

```hcl
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

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

variable "ami_id" {
  description = "AMI ID for EC2 instance. If not provided, latest Amazon Linux 2023 AMI will be used."
  type        = string
  default     = ""
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "bonfire-key"
}

variable "public_key" {
  description = "Public key material for SSH key pair (optional, will generate if empty)"
  type        = string
  default     = ""
}

variable "allowed_ssh_cidr_blocks" {
  description = "CIDR blocks allowed for SSH access"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "alarm_actions" {
  description = "List of ARNs to trigger on CloudWatch alarm (e.g. SNS topics)"
  type        = list(string)
  default     = []
}

variable "backup_s3_bucket" {
  description = "S3 bucket for game world backups"
  type        = string
}

variable "enable_eip" {
  description = "Whether to allocate and associate an Elastic IP for the instance."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}
```

**Step 2: Validate Terraform syntax**

```bash
cd terraform/modules/game-server && terraform init -backend=false && terraform validate
```

Expected: "Success! The configuration is valid."

**Step 3: Commit**

```bash
git add terraform/modules/game-server/variables.tf
git commit -m "refactor(game-server): replace Valheim-specific vars with generic game object"
```

---

## Task 3: Genericize docker-compose Template

**Files:**
- Modify: `terraform/modules/game-server/templates/docker-compose.yml.tpl`

**Step 1: Replace with generic template**

Replace the entire content of `terraform/modules/game-server/templates/docker-compose.yml.tpl` with:

```yaml
version: '3'
services:
  ${game_name}-server:
    image: ${docker_image}
    container_name: ${game_name}-server
    restart: always
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
    cap_add:
      - SYS_NICE
    volumes:
      - ${data_path}:/config
      - ${data_path}:${data_path}
```

**Step 2: Commit**

```bash
git add terraform/modules/game-server/templates/docker-compose.yml.tpl
git commit -m "refactor(game-server): genericize docker-compose template"
```

---

## Task 4: Genericize user_data Template

**Files:**
- Modify: `terraform/modules/game-server/templates/user_data.sh.tpl`

**Step 1: Replace with generic template**

Replace the entire content of `terraform/modules/game-server/templates/user_data.sh.tpl` with:

```bash
#!/bin/bash
# Install required packages
yum update -y
yum install -y docker awscli

# Enable and start Docker
systemctl enable docker
systemctl start docker

# Create directory structure
mkdir -p ${data_path}
mkdir -p /opt/${game_name}/scripts

# Create backup script
cat > /opt/${game_name}/scripts/backup.sh << 'EOBAK'
${backup_script_content}
EOBAK

# Create restore script
cat > /opt/${game_name}/scripts/restore.sh << 'EORES'
${restore_script_content}
EORES

# Make scripts executable
chmod +x /opt/${game_name}/scripts/backup.sh
chmod +x /opt/${game_name}/scripts/restore.sh

# Create Docker Compose file
cat > /opt/${game_name}/docker-compose.yml << 'EOT'
${docker_compose_content}
EOT

# Install Docker Compose
curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Setup systemd service for auto-start
cat > /etc/systemd/system/${game_name}.service << 'EOSVC'
[Unit]
Description=${display_name}
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/${game_name}
ExecStartPre=/opt/${game_name}/scripts/restore.sh
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down
ExecStop=/opt/${game_name}/scripts/backup.sh
TimeoutStartSec=0
TimeoutStopSec=300

[Install]
WantedBy=multi-user.target
EOSVC

# Enable and start game service
systemctl daemon-reload
systemctl enable ${game_name}.service
systemctl start ${game_name}.service
```

**Step 2: Commit**

```bash
git add terraform/modules/game-server/templates/user_data.sh.tpl
git commit -m "refactor(game-server): genericize user_data template"
```

---

## Task 5: Genericize backup.sh Template

**Files:**
- Modify: `terraform/modules/game-server/templates/backup.sh.tpl`

**Step 1: Replace with generic template**

Replace the entire content of `terraform/modules/game-server/templates/backup.sh.tpl` with:

```bash
#!/bin/bash
BACKUP_DIR="/tmp/${game_name}_backup"
S3_BUCKET="${s3_bucket}"
GAME_NAME="${game_name}"

echo "Starting $GAME_NAME world backup..."

# Clean up any previous backup attempts
rm -rf $BACKUP_DIR
rm -f /tmp/$${GAME_NAME}_backup.tar.gz

# Create temp backup directory
mkdir -p $BACKUP_DIR

# Copy backup paths
%{ for path in backup_paths ~}
if [ -d "${path}" ]; then
  cp -r "${path}" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy ${path}"
elif [ -f "${path}" ]; then
  cp "${path}" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy ${path}"
else
  echo "Warning: ${path} not found"
fi
%{ endfor ~}

# Check if any files were copied
if [ ! "$(ls -A $BACKUP_DIR)" ]; then
  echo "Error: No files found to backup."
  echo "Backup failed but continuing shutdown process."
else
  # Create tarball
  tar -czf "/tmp/$${GAME_NAME}_backup.tar.gz" -C "/tmp" "$${GAME_NAME}_backup"

  # Upload to S3, overwriting the existing backup
  if aws s3 cp "/tmp/$${GAME_NAME}_backup.tar.gz" "s3://$S3_BUCKET/$${GAME_NAME}_backup_latest.tar.gz"; then
    echo "Successfully updated backup in S3"
  else
    echo "Warning: Failed to upload backup to S3, but continuing shutdown process."
  fi
fi

# Cleanup local temp files
rm -rf $BACKUP_DIR /tmp/$${GAME_NAME}_backup.tar.gz
```

**Step 2: Commit**

```bash
git add terraform/modules/game-server/templates/backup.sh.tpl
git commit -m "refactor(game-server): genericize backup template"
```

---

## Task 6: Genericize restore.sh Template

**Files:**
- Modify: `terraform/modules/game-server/templates/restore.sh.tpl`

**Step 1: Replace with generic template**

Replace the entire content of `terraform/modules/game-server/templates/restore.sh.tpl` with:

```bash
#!/bin/bash
BACKUP_DIR="/tmp/${game_name}_backup"
S3_BUCKET="${s3_bucket}"
GAME_NAME="${game_name}"
DATA_PATH="${data_path}"

echo "Checking for existing $GAME_NAME data..."

# Create data directory if it doesn't exist
mkdir -p "$DATA_PATH"

# Check if data directory is empty or missing key files
if [ ! "$(ls -A $DATA_PATH 2>/dev/null)" ]; then
  echo "Data directory empty. Attempting to restore from backup..."

  # Clean up any previous restore attempts
  rm -rf $BACKUP_DIR
  rm -f /tmp/$${GAME_NAME}_backup.tar.gz

  # Download backup from S3
  if aws s3 cp "s3://$S3_BUCKET/$${GAME_NAME}_backup_latest.tar.gz" "/tmp/$${GAME_NAME}_backup.tar.gz"; then
    echo "Backup downloaded successfully"

    # Extract backup
    mkdir -p $BACKUP_DIR
    tar -xzf "/tmp/$${GAME_NAME}_backup.tar.gz" -C "/tmp"

    # Copy restored files to data path
    cp -r $BACKUP_DIR/* "$DATA_PATH/" 2>/dev/null || echo "Warning: Failed to copy restored files"

    # Set ownership (container typically runs as UID 1000)
    chown -R 1000:1000 "$DATA_PATH"

    echo "Data successfully restored from backup"

    # Cleanup
    rm -rf $BACKUP_DIR
    rm -f /tmp/$${GAME_NAME}_backup.tar.gz
  else
    echo "Warning: No backup found in S3 bucket $S3_BUCKET"
  fi
else
  echo "Existing data found. No restoration needed."
fi
```

**Step 2: Commit**

```bash
git add terraform/modules/game-server/templates/restore.sh.tpl
git commit -m "refactor(game-server): genericize restore template"
```

---

## Task 7: Update ec2.tf to Use Generic Variables

**Files:**
- Modify: `terraform/modules/game-server/ec2.tf`

**Step 1: Read current ec2.tf**

Read the file to understand current structure before modifying.

**Step 2: Update local variables and template rendering**

Find the `locals` block and template rendering sections. Update them to use the new `var.game` object:

Replace the locals block with:

```hcl
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
```

Update the `docker_compose_content` templatefile call:

```hcl
  docker_compose_content = templatefile("${path.module}/templates/docker-compose.yml.tpl", {
    game_name    = local.game_name
    docker_image = local.docker_image
    udp_ports    = local.udp_ports
    tcp_ports    = local.tcp_ports
    env_vars     = local.env_vars
    data_path    = local.data_path
  })
```

Update the `backup_script_content` templatefile call:

```hcl
  backup_script_content = templatefile("${path.module}/templates/backup.sh.tpl", {
    game_name    = local.game_name
    s3_bucket    = var.backup_s3_bucket
    backup_paths = local.backup_paths
  })
```

Update the `restore_script_content` templatefile call:

```hcl
  restore_script_content = templatefile("${path.module}/templates/restore.sh.tpl", {
    game_name = local.game_name
    s3_bucket = var.backup_s3_bucket
    data_path = local.data_path
  })
```

Update the `user_data` templatefile call:

```hcl
  user_data = templatefile("${path.module}/templates/user_data.sh.tpl", {
    game_name              = local.game_name
    display_name           = local.display_name
    data_path              = local.data_path
    docker_compose_content = local.docker_compose_content
    backup_script_content  = local.backup_script_content
    restore_script_content = local.restore_script_content
  })
```

Update the EC2 spot instance resource to use `local.instance_type` and `local.instance_name`.

Update root_block_device to use `local.volume_size`.

Update all resource names and tags to use `local.game_name` or `local.instance_name` instead of hardcoded "valheim".

**Step 3: Validate Terraform syntax**

```bash
cd terraform/modules/game-server && terraform validate
```

Expected: "Success! The configuration is valid."

**Step 4: Commit**

```bash
git add terraform/modules/game-server/ec2.tf
git commit -m "refactor(game-server): update ec2.tf to use generic game object"
```

---

## Task 8: Update security.tf for Generic Ports

**Files:**
- Modify: `terraform/modules/game-server/security.tf`

**Step 1: Read current security.tf**

Read the file to understand current structure.

**Step 2: Update security group rules to use dynamic ports**

Replace hardcoded Valheim ports (2456-2458) with dynamic rules:

```hcl
resource "aws_security_group" "game_server" {
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
```

**Step 3: Validate Terraform syntax**

```bash
cd terraform/modules/game-server && terraform validate
```

**Step 4: Commit**

```bash
git add terraform/modules/game-server/security.tf
git commit -m "refactor(game-server): use dynamic port rules in security group"
```

---

## Task 9: Update Remaining Module Files

**Files:**
- Modify: `terraform/modules/game-server/iam.tf`
- Modify: `terraform/modules/game-server/monitoring.tf`
- Modify: `terraform/modules/game-server/dashboard.tf`
- Modify: `terraform/modules/game-server/networking.tf`
- Modify: `terraform/modules/game-server/ssh.tf`
- Modify: `terraform/modules/game-server/outputs.tf`

**Step 1: Update iam.tf**

Replace hardcoded "valheim" references with `local.game_name` in resource names and descriptions.

**Step 2: Update monitoring.tf**

Replace hardcoded "valheim" references with `local.game_name` in alarm names and descriptions.

**Step 3: Update dashboard.tf**

Replace hardcoded "valheim" references with `local.game_name` in dashboard name and widget titles.

**Step 4: Update networking.tf**

Replace hardcoded "valheim" references with `local.game_name` in EIP tags.

**Step 5: Update ssh.tf**

Update key pair name to use `var.ssh_key_name` which now defaults to "bonfire-key".

**Step 6: Update outputs.tf**

Update output descriptions to be generic (not Valheim-specific).

**Step 7: Validate Terraform syntax**

```bash
cd terraform/modules/game-server && terraform validate
```

**Step 8: Commit**

```bash
git add terraform/modules/game-server/
git commit -m "refactor(game-server): genericize remaining module files"
```

---

## Task 10: Create Valheim Game Wrapper - main.tf

**Files:**
- Create: `terraform/games/valheim/main.tf`

**Step 1: Create main.tf with locals and module wiring**

Create `terraform/games/valheim/main.tf` with:

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

    data_path    = "/opt/valheim/data"
    backup_paths = ["/opt/valheim/data/worlds_local"]

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
git add terraform/games/valheim/main.tf
git commit -m "feat(valheim): create game wrapper main.tf"
```

---

## Task 11: Create Valheim Game Wrapper - variables.tf

**Files:**
- Create: `terraform/games/valheim/variables.tf`

**Step 1: Create variables.tf**

Create `terraform/games/valheim/variables.tf` with:

```hcl
# AWS Configuration
variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

# Server Configuration
variable "server_name" {
  description = "Display name of your game server"
  type        = string
  default     = "Valheim Server"
}

variable "world_name" {
  description = "Name of your Valheim world"
  type        = string
}

variable "server_pass" {
  description = "Password for accessing your server"
  type        = string
  sensitive   = true
}

variable "timezone" {
  description = "Server timezone"
  type        = string
  default     = "Europe/Stockholm"
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
  default     = "bonfire-valheim-key"
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
git add terraform/games/valheim/variables.tf
git commit -m "feat(valheim): create game wrapper variables.tf"
```

---

## Task 12: Create Valheim Game Wrapper - backend.tf and outputs.tf

**Files:**
- Create: `terraform/games/valheim/backend.tf`
- Create: `terraform/games/valheim/outputs.tf`

**Step 1: Create backend.tf**

Create `terraform/games/valheim/backend.tf` with:

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
    key     = "bonfire/valheim/terraform.tfstate"
    region  = "eu-north-1"
    encrypt = true
  }
}

provider "aws" {
  region = var.aws_region
}
```

**Step 2: Create outputs.tf**

Create `terraform/games/valheim/outputs.tf` with:

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

**Step 3: Commit**

```bash
git add terraform/games/valheim/backend.tf terraform/games/valheim/outputs.tf
git commit -m "feat(valheim): create backend.tf and outputs.tf"
```

---

## Task 13: Update discord-bot Module for Game Name

**Files:**
- Modify: `terraform/modules/discord-bot/variables.tf`
- Modify: `terraform/modules/discord-bot/main.tf`

**Step 1: Add game_name and tags variables to discord-bot module**

Add to `terraform/modules/discord-bot/variables.tf`:

```hcl
variable "game_name" {
  description = "Name of the game this bot controls"
  type        = string
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}
```

**Step 2: Update main.tf to pass game_name to Lambda**

In `terraform/modules/discord-bot/main.tf`, add `GAME_NAME` to the Lambda environment variables:

```hcl
environment {
  variables = {
    DISCORD_PUBLIC_KEY       = var.discord_public_key
    DISCORD_APPLICATION_ID   = var.discord_application_id
    DISCORD_BOT_TOKEN        = var.discord_bot_token
    AWS_REGION               = var.aws_region
    INSTANCE_ID              = var.instance_id
    AUTHORIZED_USERS         = join(",", var.discord_authorized_users)
    AUTHORIZED_ROLES         = join(",", var.discord_authorized_roles)
    GAME_NAME                = var.game_name
  }
}
```

Add `var.tags` to all resources in the module.

**Step 3: Validate**

```bash
cd terraform/modules/discord-bot && terraform init -backend=false && terraform validate
```

**Step 4: Commit**

```bash
git add terraform/modules/discord-bot/
git commit -m "feat(discord-bot): add game_name variable and tags support"
```

---

## Task 14: Update Discord Bot register-commands.js

**Files:**
- Modify: `discord_bot/register-commands.js`

**Step 1: Update commands to use /server subcommand structure**

Replace the commands array in `discord_bot/register-commands.js` with:

```javascript
const gameName = process.env.GAME_NAME || 'valheim';

const commands = [
  {
    name: 'server',
    description: 'Control game servers',
    options: [
      {
        name: gameName,
        description: `Control the ${gameName} server`,
        type: 2, // SUB_COMMAND_GROUP
        options: [
          {
            name: 'status',
            description: `Check if the ${gameName} server is running`,
            type: 1, // SUB_COMMAND
          },
          {
            name: 'start',
            description: `Start the ${gameName} server`,
            type: 1,
          },
          {
            name: 'stop',
            description: `Stop the ${gameName} server`,
            type: 1,
          },
          {
            name: 'help',
            description: `Show available commands for the ${gameName} server`,
            type: 1,
          },
        ],
      },
    ],
  },
];
```

**Step 2: Update the script to load GAME_NAME from .env**

Ensure dotenv is loaded before accessing process.env.GAME_NAME.

**Step 3: Commit**

```bash
git add discord_bot/register-commands.js
git commit -m "feat(discord-bot): update commands to /server <game> <action> structure"
```

---

## Task 15: Update Discord Bot index.js

**Files:**
- Modify: `discord_bot/src/index.js`

**Step 1: Update command parsing to handle subcommand structure**

The handler needs to parse the new structure: `/server <game> <action>`

Update the command handling section:

```javascript
const GAME_NAME = process.env.GAME_NAME || 'valheim';

// In the handler, parse subcommand groups:
if (body.type === InteractionType.APPLICATION_COMMAND) {
  const { name, options } = body.data;

  if (name === 'server') {
    // Get the subcommand group (game name)
    const gameGroup = options?.find(opt => opt.name === GAME_NAME);
    if (!gameGroup) {
      return formatResponse('Unknown game', true);
    }

    // Get the subcommand (action)
    const subcommand = gameGroup.options?.[0];
    if (!subcommand) {
      return formatResponse('Unknown action', true);
    }

    const action = subcommand.name;
    const userId = body.member?.user?.id;

    switch (action) {
      case 'status':
        return await handleStatusCommand();
      case 'start':
        return await handleStartCommand(userId);
      case 'stop':
        return await handleStopCommand(userId);
      case 'help':
        return await handleHelpCommand();
      default:
        return formatResponse('Unknown command', true);
    }
  }
}
```

**Step 2: Update help command to show new command format**

Update `handleHelpCommand()` to show `/server <game> <action>` format.

**Step 3: Commit**

```bash
git add discord_bot/src/index.js
git commit -m "feat(discord-bot): update handler for /server subcommand structure"
```

---

## Task 16: Update Discord Bot Package and Build

**Files:**
- Modify: `discord_bot/package.json`

**Step 1: Update package.json**

Rename the package and update build script output:

```json
{
  "name": "bonfire-discord-bot",
  "version": "2.0.0",
  "scripts": {
    "build": "rm -f bonfire_discord_bot.zip && npm ci --omit=dev && zip -r bonfire_discord_bot.zip node_modules src/index.js",
    "register-commands": "node register-commands.js",
    "register-commands:global": "node register-commands.js --global"
  }
}
```

**Step 2: Build the new zip**

```bash
cd discord_bot && npm run build
```

**Step 3: Add .gitignore entry if needed**

Ensure `bonfire_discord_bot.zip` is gitignored (build artifact).

**Step 4: Commit**

```bash
git add discord_bot/package.json discord_bot/.gitignore
git commit -m "chore(discord-bot): rename to bonfire and update build"
```

---

## Task 17: Create terraform.tfvars.example for Valheim

**Files:**
- Create: `terraform/games/valheim/terraform.tfvars.example`

**Step 1: Create example tfvars**

Create `terraform/games/valheim/terraform.tfvars.example`:

```hcl
# Required: Your Valheim world settings
world_name  = "MyWorld"
server_pass = "your-server-password"

# Optional: Server display name
server_name = "My Valheim Server"

# Optional: Instance configuration
# instance_type = "t3.medium"
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
git add terraform/games/valheim/terraform.tfvars.example
git commit -m "docs(valheim): add terraform.tfvars.example"
```

---

## Task 18: Validate Full Configuration

**Files:**
- All files in `terraform/games/valheim/`

**Step 1: Initialize Terraform in valheim game directory**

```bash
cd terraform/games/valheim
terraform init
```

Expected: Successful initialization

**Step 2: Create a test tfvars for validation**

```bash
cat > terraform.tfvars << 'EOF'
world_name  = "TestWorld"
server_pass = "testpass123"
EOF
```

**Step 3: Run terraform validate**

```bash
terraform validate
```

Expected: "Success! The configuration is valid."

**Step 4: Run terraform plan (dry run)**

```bash
terraform plan
```

Expected: Plan shows resources to create without errors

**Step 5: Clean up test tfvars**

```bash
rm terraform.tfvars
```

**Step 6: Commit any fixes needed**

If validation revealed issues, fix and commit.

---

## Task 19: Delete Old valheim-server Directory

**Files:**
- Delete: `terraform/valheim-server/` (entire directory)

**Step 1: Verify new structure works**

Ensure Task 18 completed successfully.

**Step 2: Delete old directory**

```bash
rm -rf terraform/valheim-server/
```

**Step 3: Commit**

```bash
git add -A
git commit -m "chore: remove old valheim-server directory (replaced by games/valheim)"
```

---

## Task 20: Update README.md

**Files:**
- Modify: `README.md`

**Step 1: Update README for Bonfire structure**

Update the README to reflect:
- New project name (Bonfire)
- New directory structure (`terraform/games/valheim/`)
- New command format (`/server valheim status`)
- How to add a new game

Key sections to update:
- Title and description
- Setup instructions (cd path)
- Discord bot commands
- Add "Adding a New Game" section

**Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README for Bonfire multi-game structure"
```

---

## Task 21: Final Verification Checklist

**Step 1: Run full terraform init and validate**

```bash
cd terraform/games/valheim
terraform init
terraform validate
```

**Step 2: Review git log**

```bash
git log --oneline -20
```

Verify all commits are in place.

**Step 3: Check directory structure**

```bash
find terraform -type f -name "*.tf" | head -30
```

Expected structure:
- `terraform/modules/game-server/*.tf`
- `terraform/modules/discord-bot/*.tf`
- `terraform/modules/s3-backup/*.tf`
- `terraform/games/valheim/*.tf`

**Step 4: Document completion**

Mark implementation as ready for real-world testing (deploying actual infrastructure).

---

## Post-Implementation: Real-World Testing

After all tasks complete, test by actually deploying:

1. Copy your existing `terraform.tfvars` to `terraform/games/valheim/`
2. Run `terraform init` (will migrate state)
3. Run `terraform plan` (should show minimal changes)
4. Run `terraform apply`
5. Connect to Valheim server to verify
6. Test Discord bot commands
7. Test backup/restore by stopping and starting instance
