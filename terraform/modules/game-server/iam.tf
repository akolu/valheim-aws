# Create IAM role for game server (CloudWatch metrics and S3 backups)
resource "aws_iam_role" "game_server_role" {
  name = "bonfire-${local.instance_name}-role"

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
    Name = "bonfire-${local.instance_name}-role"
  })
}

# Attach CloudWatch policy to the role
resource "aws_iam_role_policy_attachment" "game_server_cloudwatch" {
  role       = aws_iam_role.game_server_role.name
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
}

# Attach SSM policy to enable Session Manager shell access (no open SSH port needed)
resource "aws_iam_role_policy_attachment" "game_server_ssm" {
  role       = aws_iam_role.game_server_role.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# Create S3 backup policy
resource "aws_iam_policy" "game_server_s3_backup" {
  name        = "bonfire-${local.instance_name}-s3-backup-policy"
  description = "Policy to allow S3 access for ${local.display_name} backups"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:PutObject",
          "s3:GetObject",
          "s3:ListBucket",

          # backup.sh derives the timestamped history object from _latest with a
          # server-side `aws s3 cp`. Below the CLI's 8 MB multipart threshold that
          # is one CopyObject and S3 carries the tags across itself. At or above
          # it, the destination is created by CreateMultipartUpload — which has no
          # source to copy tags from — so the CLI reads them and re-applies them.
          # Without these two the copy fails on GetObjectTagging the moment a world
          # outgrows 8 MB, which is what froze valheim's _latest on 2026-09-11 and
          # factorio's in April.
          "s3:GetObjectTagging",
          "s3:PutObjectTagging",

          # Pruning past backup_retention_count runs `aws s3 rm`. Without this the
          # prune has silently never worked: 48 objects against a retention of 5.
          "s3:DeleteObject"
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

# Create read-only policy for long-term archive bucket (restore fallback on fresh provision)
resource "aws_iam_policy" "game_server_s3_longterm_read" {
  name        = "bonfire-${local.instance_name}-s3-longterm-read-policy"
  description = "Policy to allow read access to the long-term archive bucket for ${local.display_name} restore fallback"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "s3:GetObject",
          "s3:ListBucket"
        ]
        Effect = "Allow"
        Resource = [
          "arn:aws:s3:::${local.game_name}-long-term-backups",
          "arn:aws:s3:::${local.game_name}-long-term-backups/*"
        ]
      }
    ]
  })
}

# Attach long-term read policy to the role
resource "aws_iam_role_policy_attachment" "game_server_s3_longterm_read" {
  role       = aws_iam_role.game_server_role.name
  policy_arn = aws_iam_policy.game_server_s3_longterm_read.arn
}

# Create instance profile
resource "aws_iam_instance_profile" "game_server_profile" {
  name = "bonfire-${local.instance_name}-profile"
  role = aws_iam_role.game_server_role.name

  tags = merge(var.tags, {
    Name = "bonfire-${local.instance_name}-profile"
  })
}
