# IAM user for CloudWatch agent
resource "aws_iam_user" "cloudwatch_agent" {
  name = "valheim-cloudwatch-agent"
  path = "/"
}

# Attach CloudWatch policy to the user
resource "aws_iam_user_policy_attachment" "cloudwatch_agent_policy" {
  user       = aws_iam_user.cloudwatch_agent.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

output "cloudwatch_agent_user" {
  description = "IAM user for CloudWatch agent"
  value       = aws_iam_user.cloudwatch_agent.name
}
