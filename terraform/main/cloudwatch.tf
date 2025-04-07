# Log group for instance state events
resource "aws_cloudwatch_log_group" "instance_state_logs" {
  name              = "/aws/events/valheim-instance-state"
  retention_in_days = 30
}

# EventBridge rule for instance start events
resource "aws_cloudwatch_event_rule" "instance_started" {
  name        = "${var.instance_name}-started-alert"
  description = "Captures when the Valheim server starts"

  event_pattern = jsonencode({
    source      = ["aws.lightsail"]
    detail-type = ["AWS API Call via CloudTrail"]
    detail = {
      eventSource = ["lightsail.amazonaws.com"]
      eventName   = ["StartInstance"]
      requestParameters = {
        instanceName = [var.instance_name]
      }
    }
  })
}

# Target CloudWatch Logs for start events
resource "aws_cloudwatch_event_target" "instance_started_logs" {
  rule      = aws_cloudwatch_event_rule.instance_started.name
  target_id = "SendToCloudWatchLogs"
  arn       = aws_cloudwatch_log_group.instance_state_logs.arn
}

# EventBridge rule for instance stop events
resource "aws_cloudwatch_event_rule" "instance_stopped" {
  name        = "${var.instance_name}-stopped-alert"
  description = "Captures when the Valheim server stops"

  event_pattern = jsonencode({
    source      = ["aws.lightsail"]
    detail-type = ["AWS API Call via CloudTrail"]
    detail = {
      eventSource = ["lightsail.amazonaws.com"]
      eventName   = ["StopInstance"]
      requestParameters = {
        instanceName = [var.instance_name]
      }
    }
  })
}

# Target CloudWatch Logs for stop events
resource "aws_cloudwatch_event_target" "instance_stopped_logs" {
  rule      = aws_cloudwatch_event_rule.instance_stopped.name
  target_id = "SendToCloudWatchLogs"
  arn       = aws_cloudwatch_log_group.instance_state_logs.arn
}

resource "aws_cloudwatch_metric_alarm" "network_idle_alarm" {
  alarm_name          = "valheim-network-idle-alarm"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = var.idle_shutdown_minutes / 5 # Number of 5-minute periods
  metric_name         = "net_bytes_recv"
  namespace           = "CWAgent"
  period              = 300 # 5 minutes
  statistic           = "Sum"
  threshold           = 1000000 # 1 MB per 5 minutes
  alarm_description   = "This alarm monitors Valheim server player activity (inbound traffic)"

  dimensions = {
    interface   = "ens5",
    server_name = var.instance_name
  }
}

