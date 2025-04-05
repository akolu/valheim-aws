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
