variable "game" {
  description = "Game configuration object"
  type = object({
    name         = string
    display_name = string
    docker_image = string
    ports = object({
      udp = list(number)
      tcp = list(number)
    })
    env_vars     = map(string)
    data_path    = string
    backup_paths = list(string)
    resources = optional(object({
      instance_type = optional(string, "t3.medium")
      volume_size   = optional(number, 30)
    }), {})
  })
}

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

variable "ami_id" {
  description = "AMI ID for EC2 instance. If not provided, latest Amazon Linux 2023 AMI will be used."
  type        = string
  default     = ""
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "bonfire-key"
}

variable "public_key" {
  description = "Public key material for SSH key pair (optional, will generate if empty)"
  type        = string
  default     = ""
}

variable "allowed_ssh_cidr_blocks" {
  description = "CIDR blocks allowed for SSH access"
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "alarm_actions" {
  description = "List of ARNs to trigger on CloudWatch alarm (e.g. SNS topics)"
  type        = list(string)
  default     = []
}

variable "backup_s3_bucket" {
  description = "S3 bucket for game world backups"
  type        = string
}

variable "enable_eip" {
  description = "Whether to allocate and associate an Elastic IP for the instance."
  type        = bool
  default     = true
}

variable "tags" {
  description = "Tags to apply to all resources"
  type        = map(string)
  default     = {}
}
