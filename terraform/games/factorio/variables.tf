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
  description = "Name of the Factorio save file to create on first run"
  type        = string
  default     = "world"
}

variable "dlc_space_age" {
  description = "Enable Space Age DLC mods. Set to false if any player doesn't own the DLC."
  type        = bool
  default     = true
}

# Instance Configuration
variable "instance_type" {
  description = "EC2 instance type (t3.medium recommended for 4GB RAM)"
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

variable "discord_bot_zip_path" {
  description = "Path to Discord bot Lambda zip file"
  type        = string
  default     = "../../../discord_bot/bonfire_discord_bot.zip"
}
