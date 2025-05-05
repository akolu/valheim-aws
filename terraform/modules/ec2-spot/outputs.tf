output "instance_id" {
  description = "The ID of the EC2 spot instance"
  value       = aws_spot_instance_request.valheim_server.spot_instance_id
}

output "spot_request_id" {
  description = "ID of the spot instance request"
  value       = aws_spot_instance_request.valheim_server.id
}

output "public_ip" {
  description = "Public IP address of the Valheim server"
  value       = var.enable_eip ? aws_eip.valheim_eip[0].public_ip : aws_spot_instance_request.valheim_server.public_ip
}

output "private_key_pem" {
  description = "Private key in PEM format (only if key was generated)"
  value       = var.public_key != "" ? null : tls_private_key.ssh_key[0].private_key_pem
  sensitive   = true
}

output "ssh_key_name" {
  description = "Name of the SSH key pair"
  value       = aws_key_pair.valheim_key.key_name
}

output "security_group_id" {
  description = "ID of the security group"
  value       = aws_security_group.valheim_sg.id
}
