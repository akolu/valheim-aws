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

# discord_application_id and discord_bot_token are NOT used by Terraform itself.
# They are declared here so terraform.tfvars is a single source of credentials.
# The `bonfire bot update` CLI command reads them via cli/cmd/bot.go readBotCreds()
# to register slash commands with the Discord API.
variable "discord_application_id" {
  description = "Discord application ID (read by `bonfire bot update` CLI, not used by Terraform)"
  type        = string
}

variable "discord_bot_token" {
  description = "Discord bot token (read by `bonfire bot update` CLI, not used by Terraform)"
  type        = string
  sensitive   = true
}
