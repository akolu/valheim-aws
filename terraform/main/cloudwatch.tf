resource "aws_cloudwatch_metric_alarm" "network_idle_alarm" {
  alarm_name          = "valheim-network-idle-alarm"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = var.idle_shutdown_minutes / 5 # 5-minute periods to match idle time
  metric_name         = "NetworkOut"
  namespace           = "AWS/Lightsail"
  period              = 300 # 5 minutes
  statistic           = "Sum"
  threshold           = 1000 # Bytes per 5 minutes (very low traffic)
  alarm_description   = "This alarm monitors Valheim server network traffic"

  dimensions = {
    InstanceName = aws_lightsail_instance.valheim_server.name
  }
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
