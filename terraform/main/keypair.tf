# Generate a new private key
resource "tls_private_key" "default" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Save private key locally
resource "local_file" "private_key" {
  content  = tls_private_key.default.private_key_pem
  filename = "${var.lightsail_ssh_key_name}.pem"
}

# Create key pair in Lightsail
resource "aws_lightsail_key_pair" "valheim_key" {
  name       = var.lightsail_ssh_key_name
  public_key = tls_private_key.default.public_key_openssh
}
