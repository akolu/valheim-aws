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
