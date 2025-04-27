variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "instance_name" {
  description = "Name for the EC2 instance"
  type        = string
  default     = "valheim-server"
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.medium"
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "valheim-key-ec2"
}

variable "valheim_world_name" {
  description = "Name of your Valheim world"
  type        = string
}

variable "valheim_server_name" {
  description = "Display name of your Valheim server"
  type        = string
  default     = "Valheim Server"
}

variable "valheim_server_pass" {
  description = "Password for accessing your Valheim server"
  type        = string
  sensitive   = true
}

variable "allowed_ssh_cidr_blocks" {
  description = "CIDR blocks allowed for SSH access"
  type        = list(string)
  default     = ["0.0.0.0/0"] # For better security, restrict to your IP address
}

# Add discord bot variables

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
  description = "List of Discord user IDs authorized to control the server"
  type        = list(string)
  default     = []
}

variable "discord_authorized_roles" {
  description = "List of Discord role names authorized to control the server"
  type        = list(string)
  default     = ["Admin"]
}

variable "public_key" {
  description = "Public key material for SSH key pair (optional)"
  type        = string
  default     = ""
}

variable "discord_bot_zip_path" {
  description = "Path to the Discord bot Lambda ZIP package"
  type        = string
  default     = "../../discord_bot/valheim_discord_bot.zip"
}

variable "discord_bot_dir" {
  description = "Path to Discord bot directory containing register-commands.js"
  type        = string
  default     = "../../discord_bot"
}
