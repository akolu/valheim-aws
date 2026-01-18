# Network idle alarm for detecting player inactivity
resource "aws_cloudwatch_metric_alarm" "valheim_network_idle_alarm" {
  alarm_name          = "${var.instance_name}-network-idle"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = "6" # 30 minutes (6 × 5-minute periods)
  metric_name         = "NetworkIn"
  namespace           = "AWS/EC2"
  period              = "300" # 5 minutes
  statistic           = "Sum"
  threshold           = "1000000" # 1 MB per 5 minutes
  alarm_description   = "This alarm monitors instance network activity (inbound traffic)"

  alarm_actions = [
    "arn:aws:automate:${var.aws_region}:ec2:stop"
  ]

  dimensions = {
    InstanceId = aws_spot_instance_request.valheim_server.spot_instance_id
  }

  tags = {
    Name = "${var.instance_name}-network-idle"
  }
}

# Spot instance interruption event rule
resource "aws_cloudwatch_event_rule" "spot_interruption" {
  name        = "${var.instance_name}-spot-interruption"
  description = "Capture EC2 Spot Instance Interruption Warnings"

  event_pattern = jsonencode({
    source      = ["aws.ec2"],
    detail-type = ["EC2 Spot Instance Interruption Warning"],
    detail = {
      instance-id = [aws_spot_instance_request.valheim_server.spot_instance_id]
    }
  })

  tags = {
    Name = "${var.instance_name}-spot-interruption"
  }
}

# Optional CloudWatch Log group for spot interruption events
resource "aws_cloudwatch_log_group" "spot_interruption_logs" {
  name              = "/aws/events/${var.instance_name}-interruptions"
  retention_in_days = 30

  tags = {
    Name = "${var.instance_name}-interruptions"
  }
}

# Send spot interruption events to CloudWatch Logs
resource "aws_cloudwatch_event_target" "spot_interruption_logs" {
  rule      = aws_cloudwatch_event_rule.spot_interruption.name
  target_id = "SendToCloudWatchLogs"
  arn       = aws_cloudwatch_log_group.spot_interruption_logs.arn
}
