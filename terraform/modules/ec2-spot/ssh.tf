# Create key pair
resource "aws_key_pair" "valheim_key" {
  key_name   = var.ssh_key_name
  public_key = var.public_key != "" ? var.public_key : tls_private_key.ssh_key[0].public_key_openssh

  tags = {
    Name = var.ssh_key_name
  }
}

# Generate SSH key if not provided
resource "tls_private_key" "ssh_key" {
  count     = var.public_key != "" ? 0 : 1
  algorithm = "RSA"
  rsa_bits  = 4096
}

# Save private key to file if generated
resource "local_file" "private_key_pem" {
  count           = var.public_key != "" ? 0 : 1
  content         = tls_private_key.ssh_key[0].private_key_pem
  filename        = "${var.ssh_key_name}.pem"
  file_permission = "0400" # Secure permissions
}
