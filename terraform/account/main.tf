locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "account-hardening"
  }
}

data "aws_caller_identity" "current" {}

# Permission boundary: attached to bonfire-deploy-role to restrict IAM operations.
# The deploy role can do almost anything (PowerUserAccess) but cannot touch IAM except
# for scoped game server resources (EC2 instance profiles, CloudWatch/SSM/S3 roles).
# Broad account-level IAM changes are handled by bonfire-admin-role instead.
resource "aws_iam_policy" "deploy_permission_boundary" {
  name        = "bonfire-deploy-permission-boundary"
  description = "Permission boundary for bonfire-deploy-role: allows PowerUser actions but blocks all IAM, Organizations, and account operations"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowPowerUserActions"
        Effect = "Allow"
        NotAction = [
          "iam:*",
          "organizations:*",
          "account:*",
        ]
        Resource = "*"
      },
      {
        Sid    = "AllowPassRoleToLambda"
        Effect = "Allow"
        Action = "iam:PassRole"
        Resource = "arn:aws:iam::*:role/bonfire_*"
        Condition = {
          StringEquals = {
            "iam:PassedToService" = "lambda.amazonaws.com"
          }
        }
      },
      {
        Sid      = "AllowGameServerIAMResources"
        Effect   = "Allow"
        Action   = "iam:*"
        Resource = [
          "arn:aws:iam::*:role/bonfire-*-server-role",
          "arn:aws:iam::*:policy/bonfire-*-s3-backup-policy",
          "arn:aws:iam::*:instance-profile/bonfire-*-profile",
        ]
      },
    ]
  })

  tags = local.tags
}

# IAM role that Terraform actually uses to deploy resources.
# Granted PowerUserAccess but constrained by the permission boundary above.
resource "aws_iam_role" "deploy" {
  name                 = "bonfire-deploy-role"
  description          = "Role assumed by bonfire-base for Terraform deployments"
  permissions_boundary = aws_iam_policy.deploy_permission_boundary.arn

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowBonfireBaseToAssume"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:user/bonfire-base"
        }
        Action = "sts:AssumeRole"
      },
    ]
  })

  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "deploy_power_user" {
  role       = aws_iam_role.deploy.name
  policy_arn = "arn:aws:iam::aws:policy/PowerUserAccess"
}

# PowerUserAccess excludes iam:*, so iam:PassRole must be granted explicitly.
# Scoped to bonfire_ roles passed to Lambda only.
resource "aws_iam_role_policy" "deploy_pass_role" {
  name = "bonfire-deploy-pass-role"
  role = aws_iam_role.deploy.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "AllowPassRoleToLambda"
        Effect   = "Allow"
        Action   = "iam:PassRole"
        Resource = "arn:aws:iam::*:role/bonfire_*"
        Condition = {
          StringEquals = {
            "iam:PassedToService" = "lambda.amazonaws.com"
          }
        }
      },
    ]
  })
}

# PowerUserAccess excludes iam:*, so game server IAM actions must be granted explicitly.
# Mirrors the AllowGameServerIAMResources statement in the permission boundary — both the
# boundary AND an identity-based policy must allow an action for it to be permitted.
resource "aws_iam_role_policy" "deploy_game_server_iam" {
  name = "bonfire-deploy-game-server-iam"
  role = aws_iam_role.deploy.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "AllowGameServerIAMResources"
        Effect   = "Allow"
        Action   = "iam:*"
        Resource = [
          "arn:aws:iam::*:role/bonfire-*-server-role",
          "arn:aws:iam::*:policy/bonfire-*-s3-backup-policy",
          "arn:aws:iam::*:instance-profile/bonfire-*-profile",
        ]
      },
    ]
  })
}

# Admin role for account-level changes (IAM boundaries, deploy role modifications).
# Unlike bonfire-deploy-role, this role has full AdministratorAccess but requires MFA.
# Used only for terraform/account/ applies — never for routine infrastructure deployments.
resource "aws_iam_role" "admin" {
  name        = "bonfire-admin-role"
  description = "Role assumed by bonfire-base for account-level IAM changes; requires MFA"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowBonfireBaseToAssumeWithMFA"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:user/bonfire-base"
        }
        Action = "sts:AssumeRole"
        Condition = {
          Bool = {
            "aws:MultiFactorAuthPresent" = "true"
          }
        }
      },
    ]
  })

  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "admin_administrator" {
  role       = aws_iam_role.admin.name
  policy_arn = "arn:aws:iam::aws:policy/AdministratorAccess"
}

# IAM role for the bonfire bot Lambda function.
# Lives here (not in terraform/bot/) so that bonfire-deploy can apply terraform/bot/
# without needing IAM permissions.
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
    Name    = "bonfire_bot_lambda_role"
    Purpose = "discord-bot"
  })
}

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
        # Log group is created in terraform/bot/; use ARN pattern to avoid cross-stack reference
        Resource = "arn:aws:logs:${var.aws_region}:*:log-group:/aws/lambda/bonfire_bot:*"
      },
      {
        Sid      = "EC2Describe"
        Effect   = "Allow"
        Action   = ["ec2:DescribeInstances"]
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
        Condition = {
          StringEquals = {
            "aws:ResourceTag/Project" = "bonfire"
          }
        }
      },
      {
        Sid      = "SSMReadBonfire"
        Effect   = "Allow"
        Action   = ["ssm:GetParameter"]
        Resource = "arn:aws:ssm:*:*:parameter/bonfire/*"
      },
      {
        Sid    = "DLQSend"
        Effect = "Allow"
        Action = ["sqs:SendMessage"]
        # DLQ is created in terraform/bot/; use ARN pattern to avoid cross-stack reference
        Resource = "arn:aws:sqs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:bonfire_bot_dlq"
      },
    ]
  })
}

resource "aws_iam_role_policy_attachment" "bot_lambda" {
  role       = aws_iam_role.bot_lambda.name
  policy_arn = aws_iam_policy.bot_lambda.arn
}

# Minimal IAM user whose only capability is assuming bonfire-deploy-role and bonfire-admin-role.
# Long-lived access keys for this user go in ~/.aws/credentials as [bonfire-base].
resource "aws_iam_user" "base" {
  name = "bonfire-base"
  path = "/"
  tags = local.tags
}

resource "aws_iam_user_policy" "base_assume_roles" {
  name = "bonfire-base-assume-roles"
  user = aws_iam_user.base.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "AllowAssumeDeployRole"
        Effect   = "Allow"
        Action   = "sts:AssumeRole"
        Resource = aws_iam_role.deploy.arn
      },
      {
        Sid      = "AllowAssumeAdminRole"
        Effect   = "Allow"
        Action   = "sts:AssumeRole"
        Resource = aws_iam_role.admin.arn
      },
    ]
  })
}
