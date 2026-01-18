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
