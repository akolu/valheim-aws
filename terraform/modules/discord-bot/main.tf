# Discord Bot - Creates Lambda and API Gateway resources for controlling an EC2 instance via Discord

# IAM role for Lambda function
resource "aws_iam_role" "discord_lambda_role" {
  name = "${var.prefix}_discord_bot_lambda_role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })

  tags = {
    Name = "${var.prefix}_discord_bot_lambda_role"
  }
}

# IAM policy for Lambda function
resource "aws_iam_policy" "discord_lambda_policy" {
  name        = "${var.prefix}_discord_bot_lambda_policy"
  description = "IAM policy for Valheim Discord bot Lambda function"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Effect   = "Allow"
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Action = [
          "ec2:DescribeInstances",
          "ec2:DescribeInstanceStatus",
          "ec2:StartInstances",
          "ec2:StopInstances"
        ]
        Effect   = "Allow"
        Resource = "*"
      }
    ]
  })
}

# Attach policy to role
resource "aws_iam_role_policy_attachment" "discord_lambda_policy_attachment" {
  role       = aws_iam_role.discord_lambda_role.name
  policy_arn = aws_iam_policy.discord_lambda_policy.arn
}

# Lambda function for Discord bot
resource "aws_lambda_function" "discord_bot" {
  function_name = "${var.prefix}_discord_bot"
  role          = aws_iam_role.discord_lambda_role.arn
  handler       = "src/index.handler"
  runtime       = "nodejs20.x"
  timeout       = 15
  memory_size   = 256

  # Upload the ZIP package - using the determined path
  filename         = var.discord_bot_zip_path
  source_code_hash = filebase64sha256(var.discord_bot_zip_path)

  environment {
    variables = {
      # Discord configuration
      DISCORD_PUBLIC_KEY     = var.discord_public_key
      DISCORD_APPLICATION_ID = var.discord_application_id
      DISCORD_BOT_TOKEN      = var.discord_bot_token

      # Authorization
      AUTHORIZED_USERS = join(",", var.discord_authorized_users)
      AUTHORIZED_ROLES = join(",", var.discord_authorized_roles)

      # Server configuration
      INSTANCE_ID = var.instance_id
    }
  }

  tags = {
    Name = "${var.prefix}_discord_bot"
  }
}

# CloudWatch log group for Lambda
resource "aws_cloudwatch_log_group" "discord_lambda_logs" {
  name              = "/aws/lambda/${aws_lambda_function.discord_bot.function_name}"
  retention_in_days = 14

  tags = {
    Name = "${var.prefix}_discord_bot_logs"
  }
}

# API Gateway
resource "aws_apigatewayv2_api" "discord_api_gateway" {
  name          = "${var.prefix}-discord-bot-api"
  protocol_type = "HTTP"
  description   = "API Gateway for Valheim Discord bot"

  tags = {
    Name = "${var.prefix}-discord-bot-api"
  }
}

# API Gateway stage
resource "aws_apigatewayv2_stage" "discord_api_stage" {
  api_id      = aws_apigatewayv2_api.discord_api_gateway.id
  name        = "$default"
  auto_deploy = true
}

# API Gateway integration with Lambda
# 
# IMPORTANT: We use AWS_PROXY integration (Lambda Proxy) to ensure Discord webhook verification works.
# Discord uses Ed25519 cryptographic signatures to verify webhook requests. This verification requires:
# 1. The EXACT raw request body bytes that Discord originally signed
# 2. Unmodified headers containing the signature and timestamp
#
# AWS_PROXY passes the complete, unmodified HTTP request to Lambda, preserving the exact body bytes
# and headers needed for verification. Any transformation or parsing of the body by API Gateway
# would break the signature verification process.
#
# Without Lambda Proxy integration, API Gateway might modify the request body (even in subtle ways),
# causing the signature verification to fail because the bytes no longer match what Discord signed.
resource "aws_apigatewayv2_integration" "discord_api_integration" {
  api_id             = aws_apigatewayv2_api.discord_api_gateway.id
  integration_type   = "AWS_PROXY"
  integration_method = "POST"
  integration_uri    = aws_lambda_function.discord_bot.invoke_arn

  # Ensure payload format is not modified
  payload_format_version = "2.0"
}

# API Gateway route
resource "aws_apigatewayv2_route" "discord_api_route" {
  api_id    = aws_apigatewayv2_api.discord_api_gateway.id
  route_key = "POST /"
  target    = "integrations/${aws_apigatewayv2_integration.discord_api_integration.id}"
}

# Lambda permission for API Gateway
resource "aws_lambda_permission" "discord_api_permission" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.discord_bot.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.discord_api_gateway.execution_arn}/*/*"
}
