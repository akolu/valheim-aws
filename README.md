# Valheim Server on AWS Lightsail

A Terraform project for deploying a Valheim game server on AWS Lightsail with basic monitoring.

## Prerequisites

- AWS CLI configured with appropriate credentials
- Terraform installed
- An S3 bucket for Terraform state (valheim-tf-state)

## Setup

1. Create an S3 bucket for Terraform state:

   ```
   aws s3 mb s3://valheim-tf-state --region eu-north-1
   ```

2. Create a `terraform.tfvars` file with your sensitive values:

   ```
   cp terraform/main/terraform.tfvars.example terraform/main/terraform.tfvars
   ```

   Then edit `terraform.tfvars` to set at minimum:

   - `valheim_world_name`: Name of your Valheim world
   - `valheim_server_pass`: Password for accessing your server

3. Initialize Terraform:

   ```
   cd terraform/main
   terraform init
   ```

4. Deploy the infrastructure:
   ```
   terraform apply
   ```

## SSH Key Management

The deployment automatically generates an SSH key pair for connecting to the Lightsail instance:

- A private key file (default: `valheim-key.pem`) is created in your current directory
- Set the correct permissions on the key file: `chmod 400 valheim-key.pem`
- Use this key to SSH into your server: `ssh -i valheim-key.pem ec2-user@YOUR_SERVER_IP`

### Key Rotation

To generate a new SSH key (for security reasons or if the original is lost):

1. Change the key name via command line:

   ```
   terraform apply -var="lightsail_ssh_key_name=valheim-key-v2"
   ```

2. Or permanently update the default in `variables.tf`:

   ```terraform
   variable "lightsail_ssh_key_name" {
     default = "valheim-key-v2"  # Change from "valheim-key" to create a new key
   }
   ```

3. After applying, a new `.pem` file will be created with the new name

**Note about old keys:** When you rotate keys, the previous SSH key pairs remain registered in Lightsail but are no longer associated with the instance. You can manually delete old keys from the Lightsail console if desired (Account → SSH keys), but this isn't required. The instance will only use the most recently specified key.

## Configuration

### Server Configuration

You can customize your Valheim server by editing the following variables in your `terraform.tfvars` file:

- `valheim_server_name`: Display name of your server in the browser
- `valheim_world_name`: Name of your Valheim world
- `valheim_server_pass`: Password for accessing your server (minimum 5 characters)

Additional server settings can be modified in the Docker Compose template at `terraform/main/templates/docker-compose.yml.tpl`.

### Instance Size

Adjust the Lightsail instance size by modifying `instance_bundle_id` in your `terraform.tfvars` file. The default is `medium_3_0` (4GB RAM).

### Monitoring

Basic CloudWatch monitoring is set up to track network traffic metrics for the Valheim server.

## Usage

- Connect to your Valheim server using the Lightsail instance's public IP address.
- The Valheim server will start automatically when the instance boots.
- Monitor the server activity through the AWS CloudWatch console.
- To start/stop the server manually, use the AWS Management Console or AWS CLI:
  ```
  aws lightsail start-instance --instance-name valheim-server
  aws lightsail stop-instance --instance-name valheim-server
  ```

## Clean Up

To remove all resources:

```
terraform destroy
```
