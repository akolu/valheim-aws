data "aws_caller_identity" "current" {}

locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "discord-bot"
  }

  # IAM role managed in terraform/account/ — referenced by ARN to avoid iam:GetRole
  # (bonfire-deploy's permission boundary blocks all iam:* including read actions).
  bot_lambda_role_arn = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/bonfire_bot_lambda_role"
}

# Dead letter queue for failed async Lambda invocations (self-invoke from the
# ack path to the poll path — see docs/plans/2026-04-19-brand-bot-implementation.md
# Amendment 2). 14-day retention gives room to inspect failures manually.
resource "aws_sqs_queue" "bot_dlq" {
  name                      = "bonfire_bot_dlq"
  message_retention_seconds = 1209600 # 14 days

  tags = merge(local.tags, {
    Name = "bonfire_bot_dlq"
  })
}

# Lambda function — single Go binary handling all games
resource "aws_lambda_function" "bot" {
  function_name = "bonfire_bot"
  role          = local.bot_lambda_role_arn
  handler       = "bootstrap"
  runtime       = "provided.al2023"
  architectures = ["x86_64"]
  # 180s accommodates the synchronous EC2-poll → Discord-PATCH loop for /start
  # and /stop (see docs/plans/2026-04-19-brand-bot-implementation.md § Decision #1).
  timeout     = 180
  memory_size = 256

  # Built by discord_bot/go/Makefile — outputs to discord_bot/bonfire_discord_bot.zip
  filename         = "../../discord_bot/bonfire_discord_bot.zip"
  source_code_hash = filebase64sha256("../../discord_bot/bonfire_discord_bot.zip")

  environment {
    variables = {
      DISCORD_PUBLIC_KEY = var.discord_public_key
      # Required at runtime for the webhook PATCH URL
      # (https://discord.com/api/v10/webhooks/{app_id}/{token}/messages/@original).
      DISCORD_APP_ID = var.discord_application_id
    }
  }

  # Async-invoke failure handling is in aws_lambda_function_event_invoke_config
  # below. The old function-level `dead_letter_config` was removed (it targets
  # the same DLQ and failure class, just with older semantics) — keeping both
  # was redundant and drift-prone.

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

# Async invoke configuration — only applies to Lambda async invocations (the
# ack path's self-invoke of the poll path). Two invariants worth calling out:
#
#   maximum_retry_attempts = 0  — Architect risk #1. Default is 2 retries; a
#     transient EC2 blip during the poll would re-run the full loop and spam
#     Discord with duplicate PATCHes. We want failures to land in the DLQ,
#     not to be retried automatically.
#
#   maximum_event_age_in_seconds = 60  — don't bother dispatching polls that
#     have been queued longer than a minute; the Discord interaction token
#     ages out within 15 min and the user has likely abandoned the command.
resource "aws_lambda_function_event_invoke_config" "bot" {
  function_name                = aws_lambda_function.bot.function_name
  maximum_retry_attempts       = 0
  maximum_event_age_in_seconds = 60

  destination_config {
    on_failure {
      destination = aws_sqs_queue.bot_dlq.arn
    }
  }
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
