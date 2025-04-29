variable "bucket_name" {
  description = "Name of the S3 bucket for backups"
  type        = string
}

variable "noncurrent_version_retention_days" {
  description = "Number of days to retain noncurrent versions before deletion"
  type        = number
  default     = 7
}

variable "tags" {
  description = "Additional tags to apply to the backup resources"
  type        = map(string)
  default     = {}
}
