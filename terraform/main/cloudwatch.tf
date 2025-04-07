# Log group for instance state events
resource "aws_cloudwatch_log_group" "instance_state_logs" {
  name              = "/aws/events/valheim-instance-state"
  retention_in_days = 14
}

# Create the Lambda ZIP package with bootstrap script
data "archive_file" "lambda_zip" {
  type        = "zip"
  output_path = "${path.module}/stop_instance_lambda.zip"

  source {
    content  = "#!/bin/sh\naws lightsail stop-instance --instance-name \"${var.instance_name}\" --region \"${var.aws_region}\""
    filename = "bootstrap"
  }
}

# CloudWatch Log Group for the Lambda function
resource "aws_cloudwatch_log_group" "stop_lambda_logs" {
  name              = "/aws/lambda/stop-valheim-server"
  retention_in_days = 14
}

# Lambda function to stop the Lightsail instance
resource "aws_lambda_function" "stop_lightsail_instance" {
  function_name = "stop-valheim-server"
  role          = aws_iam_role.stop_instance_lambda_role.arn
  runtime       = "provided.al2"
  handler       = "bootstrap"
  timeout       = 30
  architectures = ["arm64"]

  depends_on       = [aws_cloudwatch_log_group.stop_lambda_logs]
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
}

# IAM role for the Lambda function
resource "aws_iam_role" "stop_instance_lambda_role" {
  name = "stop-valheim-lambda-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "lambda.amazonaws.com" }
    }]
  })
}

# IAM policy for the Lambda function
resource "aws_iam_role_policy" "stop_instance_lambda_policy" {
  name = "stop-valheim-policy"
  role = aws_iam_role.stop_instance_lambda_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Effect   = "Allow"
        Action   = "lightsail:StopInstance"
        Resource = "arn:aws:lightsail:${var.aws_region}:*:*"
      }
    ]
  })
}

# Permission for CloudWatch to invoke the Lambda
resource "aws_lambda_permission" "allow_cloudwatch" {
  statement_id  = "AllowExecutionFromCloudWatch"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.stop_lightsail_instance.function_name
  principal     = "events.amazonaws.com"
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

  alarm_actions = [
    aws_lambda_function.stop_lightsail_instance.arn
  ]
}

