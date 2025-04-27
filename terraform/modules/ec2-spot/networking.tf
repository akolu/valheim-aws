# Elastic IP for stable addressing
resource "aws_eip" "valheim_eip" {
  domain = "vpc"

  # Use lifecycle block to prevent EIP from being destroyed during update
  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Name = "${var.instance_name}-eip"
  }
}

# EIP association as a separate resource for better lifecycle management
resource "aws_eip_association" "valheim_eip_assoc" {
  instance_id   = aws_spot_instance_request.valheim_server.spot_instance_id
  allocation_id = aws_eip.valheim_eip.id
}
