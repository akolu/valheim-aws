output "valheim_server_static_ip" {
  description = "The static IP address of the Valheim server"
  value       = aws_lightsail_static_ip.valheim_static_ip.ip_address
}

output "ssh_connection_string" {
  description = "SSH connection string for the Valheim server"
  value       = "ssh -i ${var.lightsail_ssh_key_name}.pem ec2-user@${aws_lightsail_static_ip.valheim_static_ip.ip_address}"
}

output "valheim_connection_info" {
  description = "Information for connecting to the Valheim server"
  value       = "Server IP: ${aws_lightsail_static_ip.valheim_static_ip.ip_address}:2456"
}

output "ssh_private_key_path" {
  description = "Path to the SSH private key file"
  value       = "${var.lightsail_ssh_key_name}.pem has been created in your current directory"
}

output "discord_bot_url" {
  description = "The URL to use for Discord interactions endpoint"
  value       = aws_apigatewayv2_api.discord_api_gateway.api_endpoint
}
