variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "discord_public_key" {
  description = "Discord application public key for Ed25519 signature verification"
  type        = string
  sensitive   = true
}

variable "discord_application_id" {
  description = "Discord application ID (used by bonfire bot update to register slash commands)"
  type        = string
}

variable "discord_bot_token" {
  description = "Discord bot token (used by bonfire bot update to register slash commands)"
  type        = string
  sensitive   = true
}
