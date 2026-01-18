# Create IAM role for game server (CloudWatch metrics and S3 backups)
resource "aws_iam_role" "game_server_role" {
  name = "${local.instance_name}-role"

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

  tags = merge(var.tags, {
    Name = "${local.instance_name}-role"
  })
}

# Attach CloudWatch policy to the role
resource "aws_iam_role_policy_attachment" "game_server_cloudwatch" {
  role       = aws_iam_role.game_server_role.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Create S3 backup policy
resource "aws_iam_policy" "game_server_s3_backup" {
  name        = "${local.instance_name}-s3-backup-policy"
  description = "Policy to allow S3 access for ${local.display_name} backups"

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
resource "aws_iam_role_policy_attachment" "game_server_s3_backup" {
  role       = aws_iam_role.game_server_role.name
  policy_arn = aws_iam_policy.game_server_s3_backup.arn
}

# Create instance profile
resource "aws_iam_instance_profile" "game_server_profile" {
  name = "${local.instance_name}-profile"
  role = aws_iam_role.game_server_role.name

  tags = merge(var.tags, {
    Name = "${local.instance_name}-profile"
  })
}
