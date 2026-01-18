# Network idle alarm for detecting player inactivity
resource "aws_cloudwatch_metric_alarm" "game_server_network_idle" {
  alarm_name          = "${local.instance_name}-network-idle"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "6" # 30 minutes (6 x 5-minute periods)
  metric_name         = "NetworkIn"
  namespace           = "AWS/EC2"
  period              = "300" # 5 minutes
  statistic           = "Sum"
  threshold           = "1000000" # 1 MB per 5 minutes
  alarm_description   = "Monitors ${local.display_name} server network activity for auto-stop"

  alarm_actions = [
    "arn:aws:automate:${var.aws_region}:ec2:stop"
  ]

  dimensions = {
    InstanceId = aws_spot_instance_request.game_server.spot_instance_id
  }

  tags = merge(var.tags, {
    Name = "${local.instance_name}-network-idle"
  })
}

# Spot instance interruption event rule
resource "aws_cloudwatch_event_rule" "game_server_spot_interruption" {
  name        = "${local.instance_name}-spot-interruption"
  description = "Capture EC2 Spot Instance Interruption Warnings for ${local.display_name}"

  event_pattern = jsonencode({
    source      = ["aws.ec2"],
    detail-type = ["EC2 Spot Instance Interruption Warning"],
    detail = {
      instance-id = [aws_spot_instance_request.game_server.spot_instance_id]
    }
  })

  tags = merge(var.tags, {
    Name = "${local.instance_name}-spot-interruption"
  })
}

# CloudWatch Log group for spot interruption events
resource "aws_cloudwatch_log_group" "game_server_spot_interruption_logs" {
  name              = "/aws/events/${local.instance_name}-interruptions"
  retention_in_days = 30

  tags = merge(var.tags, {
    Name = "${local.instance_name}-interruptions"
  })
}

# Send spot interruption events to CloudWatch Logs
resource "aws_cloudwatch_event_target" "game_server_spot_interruption_logs" {
  rule      = aws_cloudwatch_event_rule.game_server_spot_interruption.name
  target_id = "SendToCloudWatchLogs"
  arn       = aws_cloudwatch_log_group.game_server_spot_interruption_logs.arn
}
