output "instance_id" {
  description = "ID of the EC2 instance"
  value       = module.valheim_server.instance_id
}

output "spot_request_id" {
  description = "ID of the spot instance request"
  value       = module.valheim_server.spot_request_id
}

output "public_ip" {
  description = "Public IP address of the Valheim server"
  value       = module.valheim_server.public_ip
}

output "ssh_command" {
  description = "SSH command to connect to the server"
  value       = "ssh -i ${var.ssh_key_name}.pem ec2-user@${module.valheim_server.public_ip}"
}

output "docker_logs_command" {
  description = "Command to view docker logs"
  value       = "ssh -i ${var.ssh_key_name}.pem ec2-user@${module.valheim_server.public_ip} \"cd /opt/valheim && docker-compose logs -f\""
}

output "setup_complete_message" {
  description = "Setup completion message"
  value       = "Your Valheim server has been deployed with autostart and graceful shutdown already configured. Default EC2 metrics are available in CloudWatch."
}
