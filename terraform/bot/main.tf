locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "discord-bot"
  }
}

# IAM role for the Lambda function — managed in terraform/account/ to allow
# terraform/bot/ to be applied with bonfire-deploy (which has no IAM permissions).
data "aws_iam_role" "bot_lambda" {
  name = "bonfire_bot_lambda_role"
}

# Dead letter queue for failed Lambda invocations
resource "aws_sqs_queue" "bot_dlq" {
  name = "bonfire_bot_dlq"

  tags = merge(local.tags, {
    Name = "bonfire_bot_dlq"
  })
}

# Lambda function — single Go binary handling all games
resource "aws_lambda_function" "bot" {
  function_name = "bonfire_bot"
  role          = data.aws_iam_role.bot_lambda.arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  architectures = ["x86_64"]
  timeout       = 15
  memory_size   = 256

  # Built by discord_bot/go/Makefile — outputs to discord_bot/bonfire_discord_bot.zip
  filename         = "../../discord_bot/bonfire_discord_bot.zip"
  source_code_hash = filebase64sha256("../../discord_bot/bonfire_discord_bot.zip")

  environment {
    variables = {
      DISCORD_PUBLIC_KEY = var.discord_public_key
      AWS_REGION         = var.aws_region
    }
  }

  dead_letter_config {
    target_arn = aws_sqs_queue.bot_dlq.arn
  }

  tags = merge(local.tags, {
    Name = "bonfire_bot"
  })
}

resource "aws_cloudwatch_log_group" "bot_lambda" {
  name              = "/aws/lambda/${aws_lambda_function.bot.function_name}"
  retention_in_days = 14

  tags = merge(local.tags, {
    Name = "/aws/lambda/bonfire_bot"
  })
}

# API Gateway HTTP API (v2)
resource "aws_apigatewayv2_api" "bot" {
  name          = "bonfire-bot-api"
  description   = "API Gateway for Bonfire shared Discord bot"
  protocol_type = "HTTP"

  tags = merge(local.tags, {
    Name = "bonfire-bot-api"
  })
}

# Lambda proxy integration — payload_format_version 1.0 keeps the existing
# LambdaRequest struct in main.go working without any Lambda code changes.
resource "aws_apigatewayv2_integration" "bot" {
  api_id                 = aws_apigatewayv2_api.bot.id
  integration_type       = "AWS_PROXY"
  integration_method     = "POST"
  integration_uri        = aws_lambda_function.bot.invoke_arn
  payload_format_version = "1.0"
}

# Route — Discord posts interactions to POST /
resource "aws_apigatewayv2_route" "bot_post" {
  api_id    = aws_apigatewayv2_api.bot.id
  route_key = "POST /"
  target    = "integrations/${aws_apigatewayv2_integration.bot.id}"
}

resource "aws_cloudwatch_log_group" "bot_api_access" {
  name              = "/aws/apigateway/bonfire-bot-api"
  retention_in_days = 14

  tags = merge(local.tags, {
    Name = "/aws/apigateway/bonfire-bot-api"
  })
}

# Stage — auto_deploy eliminates the need for explicit deployment resources
resource "aws_apigatewayv2_stage" "bot" {
  api_id      = aws_apigatewayv2_api.bot.id
  name        = "prod"
  auto_deploy = true

  access_log_settings {
    destination_arn = aws_cloudwatch_log_group.bot_api_access.arn
    format          = "$context.requestId $context.httpMethod $context.routeKey $context.status $context.error.message"
  }

  default_route_settings {
    throttling_rate_limit  = 10
    throttling_burst_limit = 5
  }

  tags = merge(local.tags, {
    Name = "bonfire-bot-api-prod"
  })
}

# Lambda permission allowing API Gateway v2 to invoke the function
resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.bot.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_apigatewayv2_api.bot.execution_arn}/*/*"
}
