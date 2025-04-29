# S3 bucket for secure backups with versioning and lifecycle management

resource "aws_s3_bucket" "backup" {
  bucket        = var.bucket_name
  force_destroy = false # Prevent accidental deletion during terraform destroy

  tags = merge(
    var.tags,
    {
      Name = var.bucket_name
    }
  )
}

# Enable versioning to protect against accidental deletions
resource "aws_s3_bucket_versioning" "backup_versioning" {
  bucket = aws_s3_bucket.backup.id

  versioning_configuration {
    status = "Enabled"
  }
}

# Add lifecycle rule to manage versions
resource "aws_s3_bucket_lifecycle_configuration" "backup_lifecycle" {
  bucket = aws_s3_bucket.backup.id

  rule {
    id     = "keep-previous-versions"
    status = "Enabled"

    filter {
      prefix = "" # Empty prefix means apply to all objects
    }

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_version_retention_days
    }
  }
}
