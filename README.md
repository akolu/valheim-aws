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

## Restoring an Existing Valheim World

To restore an existing world to your server:

1. **Find your local world save files**:

   - Windows:
     - `.db` files: `%USERPROFILE%\AppData\LocalLow\IronGate\Valheim\worlds\`
     - `.fwl` files: `%USERPROFILE%\AppData\LocalLow\IronGate\Valheim\worlds_local\`
   - Mac:
     - `.db` files: `~/Library/Application Support/IronGate/Valheim/worlds/`
     - `.fwl` files: `~/Library/Application Support/IronGate/Valheim/worlds_local/`
   - Linux:
     - `.db` files: `~/.config/unity3d/IronGate/Valheim/worlds/`
     - `.fwl` files: `~/.config/unity3d/IronGate/Valheim/worlds_local/`

2. **Transfer files to your Lightsail instance**:

   ```bash
   # Transfer both file types (replace with your actual paths and IP)
   scp -i valheim-key.pem /path/to/worlds/YourWorld.db ec2-user@your-instance-ip:/tmp/
   scp -i valheim-key.pem /path/to/worlds_local/YourWorld.fwl ec2-user@your-instance-ip:/tmp/
   ```

3. **SSH into the instance and move files to the correct locations**:

   ```bash
   ssh -i valheim-key.pem ec2-user@your-instance-ip

   # Create directory (both files can go in worlds_local)
   sudo mkdir -p /opt/valheim/data/worlds_local

   # Move files to worlds_local directory
   sudo mv /tmp/YourWorld.db /opt/valheim/data/worlds_local/
   sudo mv /tmp/YourWorld.fwl /opt/valheim/data/worlds_local/

   # Set correct ownership
   sudo chown -R 1000:1000 /opt/valheim/data

   # Restart the server to load the world
   cd /opt/valheim && docker-compose restart
   ```

4. **Verify the world loaded correctly**:
   ```bash
   cd /opt/valheim && docker-compose logs -f
   ```

**Note**: While the standard Valheim file structure has `.db` files in `/worlds` and `.fwl` files in `/worlds_local`, the Docker container appears to be flexible and can find both file types in the `/worlds_local` directory.

**Important**: Make sure your `valheim_world_name` in terraform.tfvars matches exactly the name of your world files (without the file extension).

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

#### Adding Memory Monitoring (Optional)

You can add memory monitoring to your running server without recreating it:

1. Create access keys for the IAM user (created via Terraform) through AWS console or CLI:

   ```bash
   # Using AWS CLI to create access keys
   aws iam create-access-key --user-name valheim-cloudwatch-agent

   # Save the AccessKeyId and SecretAccessKey displayed - you won't be able to retrieve the secret key again
   ```

2. Find your server's IP address:

   ```bash
   # From Terraform output
   terraform output valheim_server_static_ip
   ```

3. Run the monitoring installation script:

   ```bash
   # Make sure the script is executable
   chmod +x scripts/install_monitoring.sh

   # Run it with your server's details and credentials
   ./scripts/install_monitoring.sh ec2-user@YOUR_SERVER_IP valheim-key.pem ACCESS_KEY SECRET_KEY
   ```

4. The script will install and configure the CloudWatch agent to collect:

   - `mem_used_percent`: Percentage of memory used

5. View metrics in CloudWatch console:
   - Go to CloudWatch → Metrics → All metrics
   - Look for "Valheim" namespace
   - Create custom dashboards or alarms based on these metrics

## Usage

- Connect to your Valheim server using the Lightsail instance's public IP address.
- The Valheim server will start automatically when the instance boots.
- Monitor the server activity through the AWS CloudWatch console.
- To start/stop the server manually, use the AWS Management Console or AWS CLI:
  ```
  aws lightsail start-instance --instance-name valheim-server
  aws lightsail stop-instance --instance-name valheim-server
  ```

## Server Management

- **View logs**: `ssh -i valheim-key.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose logs -f"`
- **Restart server**: `ssh -i valheim-key.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose restart"`
- **Stop server**: `ssh -i valheim-key.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose stop"`

## Auto-Start Configuration

To make your Valheim server automatically start whenever the instance boots or restarts:

```bash
# Make the script executable
chmod +x scripts/setup_autostart.sh

# Run it with your server's details
./scripts/setup_autostart.sh ec2-user@YOUR_SERVER_IP valheim-key.pem
```

This creates a systemd service that will:

- Start automatically when the instance boots
- Gracefully shut down when the instance stops
- Restart the server after crashes

## Clean Up

To remove all resources:

```
terraform destroy
```
