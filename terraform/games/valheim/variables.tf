# AWS Configuration
variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "eu-north-1"
}

# Server Configuration
variable "server_name" {
  description = "Display name of your game server"
  type        = string
  default     = "Valheim Server"
}

variable "world_name" {
  description = "Name of your Valheim world"
  type        = string
}

variable "server_pass" {
  description = "Password for accessing your server"
  type        = string
  sensitive   = true
}

variable "timezone" {
  description = "Server timezone"
  type        = string
  default     = "Europe/Stockholm"
}

# Instance Configuration
variable "instance_type" {
  # t3.medium (3.75 GiB usable) is below what lloesche/valheim-server wants —
  # it logs "3.75 GiB is not enough memory" with zero players connected.
  description = "EC2 instance type"
  type        = string
  default     = "t3.large"
}

variable "volume_size" {
  description = "Root volume size in GB"
  type        = number
  default     = 30
}

variable "ssh_key_name" {
  description = "Name of the SSH key pair"
  type        = string
  default     = "bonfire-valheim-key"
}

variable "public_key" {
  description = "Public key material (optional, will generate if empty)"
  type        = string
  default     = ""
}

variable "enable_eip" {
  description = "Whether to allocate an Elastic IP"
  type        = bool
  default     = true
}

# Backup Configuration
variable "backup_retention_days" {
  description = "Number of days to retain old backup versions"
  type        = number
  default     = 7
}

