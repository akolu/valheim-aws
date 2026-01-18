# Elastic IP for stable addressing
resource "aws_eip" "game_server_eip" {
  count  = var.enable_eip ? 1 : 0
  domain = "vpc"

  tags = merge(var.tags, {
    Name = "${local.instance_name}-eip"
  })
}

# EIP association as a separate resource for better lifecycle management
resource "aws_eip_association" "game_server_eip_assoc" {
  count         = var.enable_eip ? 1 : 0
  instance_id   = aws_spot_instance_request.game_server.spot_instance_id
  allocation_id = aws_eip.game_server_eip[0].id
}
