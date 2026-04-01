variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "games" {
  description = "List of games to create long-term backup buckets for. Bucket name is derived as {game}-long-term-backups."
  type        = list(string)
  default     = ["valheim", "satisfactory", "factorio"]
}

variable "longterm_version_retention_days" {
  description = "Number of days to retain noncurrent versions in long-term buckets"
  type        = number
  default     = 90
}
