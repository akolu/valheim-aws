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

# discord_bot_token is NOT used by Terraform itself — declared here so terraform.tfvars
# is a single source of credentials. The `bonfire bot update` CLI command reads it via
# cli/cmd/bot.go readBotCreds() to register slash commands with the Discord API.
variable "discord_application_id" {
  description = "Discord application ID — wired into the Lambda's DISCORD_APP_ID env var so the bot can construct webhook PATCH URLs for deferred /start and /stop responses. Also read by `bonfire bot update` to register slash commands."
  type        = string
}

variable "discord_bot_token" {
  description = "Discord bot token (read by `bonfire bot update` CLI, not used by Terraform)"
  type        = string
  sensitive   = true
}
