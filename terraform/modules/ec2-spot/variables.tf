variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

variable "instance_name" {
  description = "Name for the EC2 instance"
  type        = string
  default     = "valheim-server"
}

variable "ami_id" {
  description = "AMI ID for EC2 instance (Amazon Linux 2023 recommended). If not provided, latest Amazon Linux 2023 AMI will be used."
  type        = string
  default     = ""
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.medium"
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "valheim-key"
}

variable "public_key" {
  description = "Public key material for SSH key pair (optional, will generate if empty)"
  type        = string
  default     = ""
}

variable "valheim_world_name" {
  description = "Name of your Valheim world"
  type        = string
}

variable "valheim_server_name" {
  description = "Display name of your Valheim server"
  type        = string
  default     = "Valheim Server"
}

variable "valheim_server_pass" {
  description = "Password for accessing your Valheim server"
  type        = string
  sensitive   = true
}

variable "allowed_ssh_cidr_blocks" {
  description = "CIDR blocks allowed for SSH access"
  type        = list(string)
  default     = ["0.0.0.0/0"] # Can be restricted by user for better security
}

variable "alarm_actions" {
  description = "List of ARNs to trigger on CloudWatch alarm (e.g. SNS topics)"
  type        = list(string)
  default     = []
}

variable "backup_s3_bucket" {
  description = "S3 bucket for Valheim world backups"
  type        = string
}
