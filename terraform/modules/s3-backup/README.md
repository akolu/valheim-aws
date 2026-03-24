# s3-backup module

Creates an S3 bucket configured for game world backups with versioning enabled and a lifecycle rule to expire old versions automatically.

## What it creates

- S3 bucket with `force_destroy = false` (prevents accidental deletion via `terraform destroy`)
- Versioning enabled to protect against accidental overwrites or deletions
- Lifecycle rule that expires noncurrent object versions after a configurable number of days

## Usage

```hcl
module "backup_bucket" {
  source = "../modules/s3-backup"

  bucket_name = "my-game-backups"
}
```

With custom retention:

```hcl
module "backup_bucket" {
  source = "../modules/s3-backup"

  bucket_name                        = "my-game-backups"
  noncurrent_version_retention_days  = 30
  tags = {
    Environment = "production"
  }
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `bucket_name` | Name of the S3 bucket to create | `string` | — | yes |
| `noncurrent_version_retention_days` | Days to retain noncurrent object versions before deletion | `number` | `7` | no |
| `tags` | Additional tags applied to backup resources | `map(string)` | `{}` | no |

## Outputs

| Name | Description |
|------|-------------|
| `bucket_name` | Name of the created S3 bucket |
| `bucket_arn` | ARN of the created S3 bucket |

## Usage with game-server module

The `game-server` module expects a backup bucket to already exist. Create this module first and pass its output:

```hcl
module "backup_bucket" {
  source      = "../modules/s3-backup"
  bucket_name = "bonfire-game-backups"
}

module "valheim" {
  source           = "../modules/game-server"
  backup_s3_bucket = module.backup_bucket.bucket_name
  # ...
}
```
