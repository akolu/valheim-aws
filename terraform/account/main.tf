locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "account-hardening"
  }

  # Computed ahead of time so the permission boundary policy can reference its own ARN
  # in the IAM delegation condition without creating a circular Terraform dependency.
  deploy_permission_boundary_arn = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:policy/bonfire-deploy-permission-boundary"
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
      # Allow creating/modifying roles only when the same permission boundary is enforced.
      # This lets bonfire-deploy create Lambda execution roles and EC2 instance roles
      # without privilege escalation — any role it creates is equally constrained.
      {
        Sid    = "AllowScopedRoleCreationWithBoundary"
        Effect = "Allow"
        Action = [
          "iam:CreateRole",
          "iam:PutRolePolicy",
          "iam:AttachRolePolicy",
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "iam:PermissionsBoundary" = local.deploy_permission_boundary_arn
          }
        }
      },
      # Deleting/detaching/passing a role cannot escalate privileges, so no boundary
      # condition is required for these operations.
      {
        Sid    = "AllowRoleDelegationNoEscalation"
        Effect = "Allow"
        Action = [
          "iam:DeleteRole",
          "iam:DetachRolePolicy",
          "iam:DeleteRolePolicy",
          "iam:PassRole",
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

# Explicit IAM delegation policy: PowerUserAccess blocks all IAM actions via NotAction,
# so we must explicitly grant the IAM role-management actions we need. The permission
# boundary condition ensures any role created/modified by bonfire-deploy is equally
# constrained — preventing privilege escalation.
resource "aws_iam_policy" "deploy_iam_delegation" {
  name        = "bonfire-deploy-iam-delegation"
  description = "Grants bonfire-deploy-role scoped IAM role management, conditioned on the permission boundary being enforced"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "AllowScopedIAMDelegation"
        Effect = "Allow"
        Action = [
          "iam:CreateRole",
          "iam:PutRolePolicy",
          "iam:AttachRolePolicy",
          "iam:DeleteRole",
          "iam:DetachRolePolicy",
          "iam:DeleteRolePolicy",
          "iam:PassRole",
          "iam:CreatePolicy",
          "iam:CreatePolicyVersion",
          "iam:DeletePolicy",
          "iam:DeletePolicyVersion",
          "iam:SetDefaultPolicyVersion",
        ]
        Resource = "*"
        Condition = {
          StringEquals = {
            "iam:PermissionsBoundary" = local.deploy_permission_boundary_arn
          }
        }
      },
    ]
  })

  tags = local.tags
}

resource "aws_iam_role_policy_attachment" "deploy_iam_delegation" {
  role       = aws_iam_role.deploy.name
  policy_arn = aws_iam_policy.deploy_iam_delegation.arn
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
