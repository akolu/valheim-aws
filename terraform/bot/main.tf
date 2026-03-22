locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "discord-bot"
  }
}

# IAM role for the Lambda function
resource "aws_iam_role" "bot_lambda" {
  name = "bonfire_bot_lambda_role"

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

  tags = merge(local.tags, {
    Name = "bonfire_bot_lambda_role"
  })
}

# IAM policy granting the Lambda permissions to read EC2, start/stop instances, and read SSM
resource "aws_iam_policy" "bot_lambda" {
  name        = "bonfire_bot_lambda_policy"
  description = "IAM policy for Bonfire shared Discord bot Lambda"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "CloudWatchLogs"
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents",
        ]
        Resource = "arn:aws:logs:*:*:*"
      },
      {
        Sid    = "EC2Describe"
        Effect = "Allow"
        Action = ["ec2:DescribeInstances"]
        Resource = "*"
      },
      {
        Sid    = "EC2StartStop"
        Effect = "Allow"
        Action = [
          "ec2:StartInstances",
          "ec2:StopInstances",
        ]
        Resource = "*"
      },
      {
        Sid    = "SSMReadBonfire"
        Effect = "Allow"
        Action = ["ssm:GetParameter"]
        Resource = "arn:aws:ssm:*:*:parameter/bonfire/*"
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "bot_lambda" {
  role       = aws_iam_role.bot_lambda.name
  policy_arn = aws_iam_policy.bot_lambda.arn
}

# Lambda function — single Go binary handling all games
resource "aws_lambda_function" "bot" {
  function_name = "bonfire_bot"
  role          = aws_iam_role.bot_lambda.arn
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

# API Gateway REST API
resource "aws_api_gateway_rest_api" "bot" {
  name        = "bonfire-bot-api"
  description = "API Gateway for Bonfire shared Discord bot"

  tags = merge(local.tags, {
    Name = "bonfire-bot-api"
  })
}

# POST method on the root resource — Discord posts interactions to /
resource "aws_api_gateway_method" "bot_post" {
  rest_api_id   = aws_api_gateway_rest_api.bot.id
  resource_id   = aws_api_gateway_rest_api.bot.root_resource_id
  http_method   = "POST"
  authorization = "NONE"
}

# Lambda proxy integration — passes the raw request body and headers to Lambda unchanged,
# which is required for Discord's Ed25519 signature verification to work correctly.
resource "aws_api_gateway_integration" "bot" {
  rest_api_id             = aws_api_gateway_rest_api.bot.id
  resource_id             = aws_api_gateway_rest_api.bot.root_resource_id
  http_method             = aws_api_gateway_method.bot_post.http_method
  integration_http_method = "POST"
  type                    = "AWS_PROXY"
  uri                     = aws_lambda_function.bot.invoke_arn
}

# Deployment — triggers re-deploy whenever the method or integration changes
resource "aws_api_gateway_deployment" "bot" {
  rest_api_id = aws_api_gateway_rest_api.bot.id

  depends_on = [
    aws_api_gateway_method.bot_post,
    aws_api_gateway_integration.bot,
  ]

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_api_gateway_stage" "bot" {
  deployment_id = aws_api_gateway_deployment.bot.id
  rest_api_id   = aws_api_gateway_rest_api.bot.id
  stage_name    = "prod"

  tags = merge(local.tags, {
    Name = "bonfire-bot-api-prod"
  })
}

# Lambda permission allowing API Gateway to invoke the function
resource "aws_lambda_permission" "api_gateway" {
  statement_id  = "AllowAPIGatewayInvoke"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.bot.function_name
  principal     = "apigateway.amazonaws.com"
  source_arn    = "${aws_api_gateway_rest_api.bot.execution_arn}/*/*"
}
