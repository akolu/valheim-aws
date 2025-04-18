variable "aws_region" {
  description = "AWS region to deploy resources (default: Stockholm)"
  type        = string
  default     = "eu-north-1"
}

variable "instance_name" {
  description = "Name for the Lightsail instance"
  type        = string
  default     = "valheim-server"
}

variable "instance_blueprint_id" {
  description = "Lightsail blueprint ID (OS)"
  type        = string
  default     = "amazon_linux_2023"
}

variable "instance_bundle_id" {
  description = "Lightsail bundle ID (instance size)"
  type        = string
  default     = "medium_3_0"
}

variable "idle_shutdown_minutes" {
  description = "Time period for CloudWatch alarm evaluation (minutes)"
  type        = number
  default     = 30
}

variable "lightsail_ssh_key_name" {
  description = "Name for the Lightsail SSH key pair. To rotate keys, change this value (e.g., from 'valheim-key' to 'valheim-key-v2')"
  type        = string
  default     = "valheim-key"
}

variable "valheim_world_name" {
  description = "Name of the Valheim world"
  type        = string
  sensitive   = true
}

variable "valheim_server_name" {
  description = "Display name of the Valheim server"
  type        = string
  default     = "MyValheimServer"
}

variable "valheim_server_pass" {
  description = "Password for the Valheim server"
  type        = string
  sensitive   = true
}

# Discord Bot Configuration
variable "discord_bot_zip_path" {
  description = "Path to the Discord bot Lambda ZIP package"
  type        = string
  default     = "../../discord_bot/valheim_discord_bot.zip"
}

variable "discord_authorized_users" {
  description = "List of Discord user IDs authorized to control the server"
  type        = list(string)
  default     = []
}

variable "discord_authorized_roles" {
  description = "List of Discord role names authorized to control the server"
  type        = list(string)
  default     = ["Admin"]
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
