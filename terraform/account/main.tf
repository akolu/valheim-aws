locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "account-hardening"
  }
}

data "aws_caller_identity" "current" {}

# Permission boundary: attached to bonfire-deploy-role to block all IAM operations.
# The deploy role can do almost anything (PowerUserAccess) but cannot touch IAM at all.
# Account-level IAM changes are handled by bonfire-admin-role instead.
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
          BoolIfExists = {
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
