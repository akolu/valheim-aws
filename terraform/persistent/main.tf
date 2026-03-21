locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "longterm-backups"
  }

  longterm_buckets = {
    valheim      = "valheim-long-term-backups"
    satisfactory = "satisfactory-long-term-backups"
  }
}

resource "aws_s3_bucket" "longterm" {
  for_each = local.longterm_buckets

  bucket        = each.value
  force_destroy = false

  tags = merge(local.tags, {
    Name = each.value
    Game = each.key
  })
}

resource "aws_s3_bucket_versioning" "longterm" {
  for_each = local.longterm_buckets

  bucket = aws_s3_bucket.longterm[each.key].id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "longterm" {
  for_each = local.longterm_buckets

  bucket = aws_s3_bucket.longterm[each.key].id

  rule {
    id     = "retain-noncurrent-versions"
    status = "Enabled"

    filter {
      prefix = ""
    }

    noncurrent_version_expiration {
      noncurrent_days = var.longterm_version_retention_days
    }
  }
}
