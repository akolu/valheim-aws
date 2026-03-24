# game-server module

Provisions a containerized game server on an EC2 spot instance with automated backups to S3, CloudWatch monitoring, and SSM-based shell access.

## What it creates

- EC2 spot instance (persistent, stops on interruption rather than terminating)
- Latest Amazon Linux 2023 AMI (or caller-supplied AMI)
- Optional Elastic IP for a stable public address
- Security group with configurable game ports and optional SSH break-glass
- IAM role with CloudWatch, SSM Session Manager, and S3 backup permissions
- Read-only IAM policy for long-term archive bucket (restore fallback)
- SSH key pair (generated or caller-supplied)
- CloudWatch dashboard for instance metrics
- User data that installs Docker, writes a docker-compose file, and configures backup/restore scripts as systemd timers

## Usage

```hcl
module "valheim" {
  source = "../modules/game-server"

  game = {
    name         = "valheim"
    display_name = "Valheim"
    docker_image = "lloesche/valheim-server:latest"
    ports = {
      udp = [2456, 2457, 2458]
      tcp = []
    }
    env_vars = {
      SERVER_NAME = "My Server"
      WORLD_NAME  = "MyWorld"
    }
    data_path    = "/opt/valheim/data"
    backup_paths = ["/opt/valheim/data/worlds"]
  }

  backup_s3_bucket = "my-game-backups"
}
```

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|----------|
| `game` | Game configuration object (see below) | `object` | — | yes |
| `backup_s3_bucket` | S3 bucket name for game world backups | `string` | — | yes |
| `aws_region` | AWS region to deploy resources | `string` | `"eu-north-1"` | no |
| `ami_id` | AMI ID for the EC2 instance. Latest Amazon Linux 2023 used if empty. | `string` | `""` | no |
| `ssh_key_name` | Name of the SSH key pair | `string` | `"bonfire-key"` | no |
| `public_key` | Public key material for SSH key pair. A key is generated if empty. | `string` | `""` | no |
| `allowed_ssh_cidr_blocks` | CIDR blocks allowed for SSH. Empty by default — use SSM for normal access. | `list(string)` | `[]` | no |
| `backup_retention_count` | Number of timestamped backups to retain in S3 | `number` | `5` | no |
| `enable_eip` | Allocate and associate an Elastic IP | `bool` | `true` | no |
| `tags` | Tags applied to all resources | `map(string)` | `{}` | no |

### game object

| Field | Description | Type | Required |
|-------|-------------|------|----------|
| `name` | Short identifier used in resource names (e.g. `valheim`) | `string` | yes |
| `display_name` | Human-readable name used in tags and descriptions | `string` | yes |
| `docker_image` | Docker image to run | `string` | yes |
| `ports.udp` | UDP ports to open in the security group | `list(number)` | yes |
| `ports.tcp` | TCP ports to open in the security group | `list(number)` | yes |
| `env_vars` | Environment variables passed to the container | `map(string)` | yes |
| `data_path` | Path on the instance where game data is stored | `string` | yes |
| `backup_paths` | Paths included in backup archives | `list(string)` | yes |
| `init_service` | Optional sidecar container run once before the game starts | `object` | no |
| `resources.instance_type` | EC2 instance type | `string` | no (`t3.medium`) |
| `resources.volume_size` | Root EBS volume size in GiB | `number` | no (`30`) |

### init_service object

| Field | Description | Type |
|-------|-------------|------|
| `image` | Docker image for the init container | `string` |
| `command` | Command to run | `string` |
| `env_vars` | Environment variables for the init container | `map(string)` |

## Outputs

| Name | Description |
|------|-------------|
| `instance_id` | EC2 spot instance ID |
| `spot_request_id` | Spot instance request ID |
| `public_ip` | Public IP address (Elastic IP if `enable_eip = true`) |
| `private_key_pem` | Generated SSH private key in PEM format (sensitive; `null` if `public_key` was supplied) |
| `ssh_key_name` | Name of the AWS key pair |
| `security_group_id` | ID of the game server security group |
| `ssh_command` | Ready-to-use SSH command string |

## Shell access

Normal access uses **SSM Session Manager** — no open SSH port required:

```bash
aws ssm start-session --target <instance_id>
```

Break-glass SSH is available by passing `allowed_ssh_cidr_blocks`:

```hcl
allowed_ssh_cidr_blocks = ["203.0.113.10/32"]
```

## IAM permissions granted to the instance

- `CloudWatchAgentServerPolicy` — publish metrics and logs
- `AmazonSSMManagedInstanceCore` — SSM Session Manager access
- S3 read/write on `backup_s3_bucket` (put, get, list)
- S3 read-only on `<game_name>-long-term-backups` bucket (restore fallback)
