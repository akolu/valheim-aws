# Bonfire - Game Server on AWS

A Terraform project for deploying dedicated game servers on AWS using EC2 Spot Instances. Currently supports Valheim, with an extensible architecture for adding more games.

## Prerequisites

- AWS CLI configured with appropriate credentials
- Terraform installed
- An S3 bucket for Terraform state

## Setup

1. Create an S3 bucket for Terraform state:

   ```
   aws s3 mb s3://bonfire-tf-state --region eu-north-1
   ```

2. Navigate to the game directory:

   ```
   cd terraform/games/valheim
   ```

3. Create a `terraform.tfvars` file with your sensitive values:

   ```
   cp terraform.tfvars.example terraform.tfvars
   ```

   Then edit `terraform.tfvars` to set at minimum:

   - `world_name`: Name of your game world
   - `server_pass`: Password for accessing your server

4. Initialize Terraform:

   ```
   terraform init
   ```

5. Deploy the infrastructure:
   ```
   terraform apply
   ```

## Migrating from Previous Version

If you have an existing deployment using the old `terraform/valheim-server/` structure, migrate your Terraform state before initializing the new structure:

```bash
# Copy the state file to the new location
aws s3 cp s3://valheim-ec2-tf-state/terraform.tfstate \
          s3://valheim-ec2-tf-state/bonfire/valheim/terraform.tfstate

# Copy your existing tfvars
cp terraform/valheim-server/terraform.tfvars terraform/games/valheim/terraform.tfvars

# Initialize in the new directory
cd terraform/games/valheim
terraform init

# Verify the state was picked up (should show existing resources)
terraform plan
```

**Note:** Update variable names in your tfvars if needed:
- `valheim_world_name` → `world_name`
- `valheim_server_name` → `server_name`
- `valheim_server_pass` → `server_pass`

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

2. **Transfer files to your EC2 instance**:

   ```bash
   # Transfer both file types (replace with your actual paths and IP)
   scp -i valheim-key-ec2.pem /path/to/worlds/YourWorld.db ec2-user@your-instance-ip:/tmp/
   scp -i valheim-key-ec2.pem /path/to/worlds_local/YourWorld.fwl ec2-user@your-instance-ip:/tmp/
   ```

3. **SSH into the instance and move files to the correct locations**:

   ```bash
   ssh -i valheim-key-ec2.pem ec2-user@your-instance-ip

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

**Important**: Make sure your `world_name` in terraform.tfvars matches exactly the name of your world files (without the file extension).

## SSH Key Management

The deployment automatically generates an SSH key pair for connecting to the EC2 instance:

- A private key file (default: `valheim-key-ec2.pem`) is created in your current directory
- Set the correct permissions on the key file: `chmod 400 valheim-key-ec2.pem`
- Use this key to SSH into your server: `ssh -i valheim-key-ec2.pem ec2-user@YOUR_SERVER_IP`

## Configuration

### Server Configuration

You can customize your server by editing the following variables in your `terraform.tfvars` file:

- `server_name`: Display name of your server in the browser
- `world_name`: Name of your game world
- `server_pass`: Password for accessing your server (minimum 5 characters for Valheim)

### Instance Type

You can adjust the EC2 instance type in your `terraform.tfvars` file. The default is `t3.medium`.

### Monitoring

Basic CloudWatch monitoring is set up to track network traffic metrics for the game server. A CloudWatch dashboard is automatically created to monitor your instance.

## Usage

- Connect to your game server using the EC2 instance's public IP address (provided in the Terraform output).
- The game server will start automatically when the instance boots.
- Monitor the server activity through the AWS CloudWatch console.
- To start/stop the server manually, use the AWS Management Console or AWS CLI:
  ```
  aws ec2 start-instances --instance-ids i-1234567890abcdef0
  aws ec2 stop-instances --instance-ids i-1234567890abcdef0
  ```

## Server Management

- **View logs**: `ssh -i valheim-key-ec2.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose logs -f"`
- **Restart server**: `ssh -i valheim-key-ec2.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose restart"`
- **Stop server**: `ssh -i valheim-key-ec2.pem ec2-user@YOUR_IP "cd /opt/valheim && docker-compose stop"`

## Discord Bot Integration

A serverless Discord bot is provided that allows your play group to control the game server directly from Discord with slash commands. It's implemented using AWS Lambda and API Gateway for virtually no cost.

### Bot Commands

Commands use the format `/<game> <action>`:

- `/<game> status` - Check if the server is running or stopped
- `/<game> start` - Start the server (authorized users only)
- `/<game> stop` - Stop the server (authorized users only)
- `/<game> help` - Show available commands

For example: `/valheim start`, `/satisfactory status`

### Setup Instructions

See the [Discord Bot README](discord_bot/README.md) for detailed setup and deployment instructions.

## Adding a New Game

To add support for a new game:

1. **Copy an existing game directory**:
   ```bash
   cp -r terraform/games/valheim terraform/games/satisfactory
   ```

2. **Update `terraform/games/satisfactory/main.tf`**:
   - Change `local.game` to define the new game's Docker image, ports, and environment variables
   - Update any game-specific configuration

3. **Update `terraform/games/satisfactory/variables.tf`**:
   - Adjust variable names and defaults for the new game
   - Add any game-specific variables needed

4. **Update `terraform/games/satisfactory/backend.tf`**:
   - Change the state key to `bonfire/satisfactory/terraform.tfstate`

5. **Deploy**:
   ```bash
   cd terraform/games/satisfactory
   terraform init
   terraform apply
   ```

## Instance State Alerts

CloudWatch Event Rules are configured to track when your game server instance starts or stops. These events are captured and can be used for logging and monitoring purposes.

The alert rules can be found in the CloudWatch console under "Rules" and could be used as triggers for custom actions in the future.

## Spot Instance Interruption Handling

The EC2 spot instance is configured with interruption behavior set to "stop" rather than "terminate". This means:

1. If AWS needs to reclaim the capacity, your instance will be stopped instead of terminated
2. All your data will be preserved
3. When capacity becomes available again, you can restart the instance
4. Interruption warnings are logged to CloudWatch

## Prerequisites

### CloudTrail

The instance state alerts rely on CloudTrail to capture API activity. CloudTrail should be enabled in your AWS account to make these alerts work.

> **Note:** CloudTrail is an account-wide service and only needs to be set up once per AWS account, not per project.

If you don't have CloudTrail enabled yet, you can set it up through the AWS console:

1. Go to the CloudTrail console
2. Click "Create trail"
3. Name your trail (e.g., "management-events")
4. Select "Create new S3 bucket" (or use an existing one)
5. Enable for all regions
6. Choose "Management events" at minimum
7. Click "Next" and "Create trail"

Alternatively, you can enable it via the AWS CLI:

```bash
# Create a bucket for CloudTrail logs
aws s3api create-bucket --bucket cloudtrail-logs-ACCOUNT_ID --region YOUR_REGION --create-bucket-configuration LocationConstraint=YOUR_REGION

# Add a bucket policy for CloudTrail
aws s3api put-bucket-policy --bucket cloudtrail-logs-ACCOUNT_ID --policy file://bucket-policy.json

# Create and start the trail
aws cloudtrail create-trail --name management-events --s3-bucket-name cloudtrail-logs-ACCOUNT_ID --is-multi-region-trail --enable-log-file-validation
aws cloudtrail start-logging --name management-events
```

Once CloudTrail is enabled, the EventBridge rules will automatically start capturing game server state changes.

## Clean Up

To remove all resources:

```bash
terraform destroy
```
