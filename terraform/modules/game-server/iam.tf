# Create IAM role for CloudWatch metrics
resource "aws_iam_role" "cloudwatch_role" {
  name = "${var.instance_name}-cloudwatch-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      },
    ]
  })

  tags = {
    Name = "${var.instance_name}-cloudwatch-role"
  }
}

# Attach CloudWatch policy to the role
resource "aws_iam_role_policy_attachment" "cloudwatch_attachment" {
  role       = aws_iam_role.cloudwatch_role.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Create S3 backup policy
resource "aws_iam_policy" "s3_backup_policy" {
  name        = "${var.instance_name}-s3-backup-policy"
  description = "Policy to allow S3 access for Valheim world backups"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:ListBucket"
        ]
        Effect = "Allow"
        Resource = [
          "arn:aws:s3:::${var.backup_s3_bucket}",
          "arn:aws:s3:::${var.backup_s3_bucket}/*"
        ]
      }
    ]
  })
}

# Attach S3 backup policy to the role
resource "aws_iam_role_policy_attachment" "s3_backup_attachment" {
  role       = aws_iam_role.cloudwatch_role.name
  policy_arn = aws_iam_policy.s3_backup_policy.arn
}

# Create instance profile
resource "aws_iam_instance_profile" "cloudwatch_profile" {
  name = "${var.instance_name}-cloudwatch-profile"
  role = aws_iam_role.cloudwatch_role.name

  tags = {
    Name = "${var.instance_name}-cloudwatch-profile"
  }
}
