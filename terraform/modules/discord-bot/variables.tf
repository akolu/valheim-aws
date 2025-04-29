variable "prefix" {
  description = "Prefix for naming resources"
  type        = string
  default     = "valheim"
}

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

variable "instance_id" {
  description = "ID of the EC2 instance to control"
  type        = string
}

# Discord Bot Configuration
variable "discord_bot_zip_path" {
  description = "Path to the Discord bot Lambda ZIP package"
  type        = string
}

variable "discord_authorized_users" {
  description = "List of Discord user IDs authorized to control the server"
  type        = list(string)
  default     = []
}

variable "discord_authorized_roles" {
  description = "List of Discord role names authorized to control the server"
  type        = list(string)
  default     = []
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

variable "discord_bot_dir" {
  description = "Path to Discord bot directory containing register-commands.js"
  type        = string
  default     = "../../discord_bot"
}
