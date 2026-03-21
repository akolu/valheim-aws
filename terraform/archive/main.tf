locals {
  tags = {
    Project   = "bonfire"
    ManagedBy = "terraform"
    Purpose   = "longterm-backups"
  }
}

resource "aws_s3_bucket" "longterm" {
  for_each = toset(var.games)

  bucket = "${each.key}-long-term-backups"

  lifecycle {
    prevent_destroy = true
  }

  tags = merge(local.tags, {
    Name = "${each.key}-long-term-backups"
    Game = each.key
  })
}

resource "aws_s3_bucket_versioning" "longterm" {
  for_each = toset(var.games)

  bucket = aws_s3_bucket.longterm[each.key].id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "longterm" {
  for_each = toset(var.games)

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
