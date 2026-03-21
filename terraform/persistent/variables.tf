variable "aws_region" {
  description = "AWS region"
  type        = string
  default     = "eu-north-1"
}

variable "longterm_version_retention_days" {
  description = "Number of days to retain noncurrent versions in long-term buckets"
  type        = number
  default     = 90
}
