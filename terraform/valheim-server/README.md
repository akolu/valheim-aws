# Valheim Server on AWS EC2

This is a Terraform module for deploying a Valheim game server on an AWS EC2 spot instance.

## Benefits over Lightsail

- Cost savings: EC2 spot instances can be 50-90% cheaper than on-demand instances
- More flexibility in instance types
- Better performance options when needed

## Prerequisites

- AWS CLI configured with appropriate credentials
- Terraform installed
- An S3 bucket for Terraform state (valheim-tf-state)

## Automated Features

This module includes several automated features that are set up during instance creation:

1. **Automatic Server Start** - Systemd service that:

   - Starts the Valheim server automatically when the instance boots
   - Gracefully stops the server when the instance is shut down

2. **Built-in EC2 Monitoring** - Standard CloudWatch metrics including:

   - CPU utilization
   - Network throughput
   - Disk I/O performance
   - Status checks
   - All metrics available in the CloudWatch console under "AWS/EC2" namespace

## Setup

1. Create a `terraform.tfvars` file with your sensitive values:

   ```
   cp terraform.tfvars.example terraform.tfvars
   ```

   Then edit `terraform.tfvars` to set at minimum:

   - `valheim_world_name`: Name of your Valheim world
   - `valheim_server_pass`: Password for accessing your server

2. Initialize Terraform:

   ```
   terraform init
   ```

3. Deploy the infrastructure:
   ```
   terraform apply
   ```

## EC2 Spot Instance Notes

- Automatically uses on-demand price as maximum bid (optimal strategy)
- The instance is configured as a persistent spot request, meaning it will restart when interrupted
- It starts with a small instance type (t3.medium) for testing, and can be scaled up later
- Spot interruptions are logged to CloudWatch Logs for monitoring
- An Elastic IP is attached to provide a stable public IP address

## Connecting to the Server

After deployment, the outputs will show:

- The server's IP address
- SSH command to connect to the server
- Command to view Docker logs

## Restoring an Existing Valheim World

Follow the same steps as described in the main README:

1. **Find your local world save files**
2. **Transfer files to your EC2 instance** (replacing the instance IP)
3. **SSH into the instance and move files to the correct locations**
4. **Restart the server to load the world**

## Customization

You can adjust settings in `terraform.tfvars`:

- `instance_type`: Change to a larger instance when needed (e.g., m5a.large, r5a.large)
- Valheim server settings (name, password, etc.)
