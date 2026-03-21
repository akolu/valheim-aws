output "deploy_role_arn" {
  description = "ARN of bonfire-deploy-role (use as role_arn in AWS config profile)"
  value       = aws_iam_role.deploy.arn
}

output "base_user_arn" {
  description = "ARN of bonfire-base IAM user"
  value       = aws_iam_user.base.arn
}

output "permission_boundary_arn" {
  description = "ARN of the deploy permission boundary policy"
  value       = aws_iam_policy.deploy_permission_boundary.arn
}
