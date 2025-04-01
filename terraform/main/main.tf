resource "aws_lightsail_instance" "valheim_server" {
  depends_on        = [aws_lightsail_key_pair.valheim_key]
  name              = var.instance_name
  availability_zone = "${var.aws_region}a"
  blueprint_id      = var.instance_blueprint_id
  bundle_id         = var.instance_bundle_id
  key_pair_name     = var.lightsail_ssh_key_name
  ip_address_type   = "ipv4"

  user_data = <<-EOF
    #!/bin/bash
    dnf install -y docker
    systemctl enable docker
    systemctl start docker
    
    mkdir -p /opt/valheim
    cat > /opt/valheim/docker-compose.yml << 'EOT'
${local.docker_compose}
EOT
    
    curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
    
    cd /opt/valheim && docker-compose up -d
  EOF

  tags = {
    Name = var.instance_name
  }
}

# Create a static IP
resource "aws_lightsail_static_ip" "valheim_static_ip" {
  name = "${var.instance_name}-static-ip"
}

# Attach the static IP to the instance
resource "aws_lightsail_static_ip_attachment" "valheim_static_ip_attachment" {
  static_ip_name = aws_lightsail_static_ip.valheim_static_ip.name
  instance_name  = aws_lightsail_instance.valheim_server.name
}

resource "aws_lightsail_instance_public_ports" "valheim_ports" {
  instance_name = aws_lightsail_instance.valheim_server.name

  port_info {
    protocol  = "udp"
    from_port = 2456
    to_port   = 2458
  }

  port_info {
    protocol  = "tcp"
    from_port = 22
    to_port   = 22
  }
}
