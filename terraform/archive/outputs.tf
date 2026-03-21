output "longterm_bucket_names" {
  description = "Long-term backup bucket names by game"
  value       = { for k, v in aws_s3_bucket.longterm : k => v.bucket }
}

output "longterm_bucket_arns" {
  description = "Long-term backup bucket ARNs by game"
  value       = { for k, v in aws_s3_bucket.longterm : k => v.arn }
}
