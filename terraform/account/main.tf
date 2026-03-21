locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "account-hardening"
  }
}

data "aws_caller_identity" "current" {}

# Permission boundary: attached to bonfire-deploy-role to prevent IAM escalation.
# The deploy role can do almost anything (PowerUserAccess) but cannot create/modify
# IAM users, groups, or attach broad policies — stopping privilege escalation.
resource "aws_iam_policy" "deploy_permission_boundary" {
  name        = "bonfire-deploy-permission-boundary"
  description = "Permission boundary for bonfire-deploy-role: allows PowerUser actions but blocks IAM escalation vectors"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowPowerUserActions"
        Effect = "Allow"
        NotAction = [
          "iam:CreateUser",
          "iam:DeleteUser",
          "iam:AttachUserPolicy",
          "iam:DetachUserPolicy",
          "iam:PutUserPolicy",
          "iam:DeleteUserPolicy",
          "iam:AddUserToGroup",
          "iam:RemoveUserFromGroup",
          "iam:CreateGroup",
          "iam:DeleteGroup",
          "iam:AttachGroupPolicy",
          "iam:DetachGroupPolicy",
          "iam:PutGroupPolicy",
          "iam:DeleteGroupPolicy",
          "iam:CreateRole",
          "iam:DeleteRole",
          "iam:AttachRolePolicy",
          "iam:DetachRolePolicy",
          "iam:PutRolePolicy",
          "iam:DeleteRolePolicy",
          "iam:UpdateAssumeRolePolicy",
          "iam:CreatePolicy",
          "iam:DeletePolicy",
          "iam:CreatePolicyVersion",
          "iam:DeletePolicyVersion",
          "iam:SetDefaultPolicyVersion",
          "iam:PassRole",
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

# Minimal IAM user whose only capability is assuming bonfire-deploy-role.
# Long-lived access keys for this user go in ~/.aws/credentials as [bonfire-base].
resource "aws_iam_user" "base" {
  name = "bonfire-base"
  path = "/"
  tags = local.tags
}

resource "aws_iam_user_policy" "base_assume_deploy" {
  name = "bonfire-base-assume-deploy-role"
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
    ]
  })
}
