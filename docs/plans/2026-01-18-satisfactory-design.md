# Satisfactory Game Server Design

## Overview

Add Satisfactory as a new game to the Bonfire platform by creating a thin wrapper at `terraform/games/satisfactory/` that configures the existing `game-server` module.

## Game Configuration

```hcl
game = {
  name         = "satisfactory"
  display_name = "Satisfactory Server"
  docker_image = "wolveix/satisfactory-server:latest"

  ports = {
    udp = [7777]
    tcp = []
  }

  env_vars = {
    MAXPLAYERS = "4"
  }

  data_path    = "/config"
  backup_paths = ["/config/saved"]

  resources = {
    instance_type = "t3.large"   # 8GB RAM (Satisfactory requirement)
    volume_size   = 30
  }
}
```

### Key Differences from Valheim

| Aspect | Valheim | Satisfactory |
|--------|---------|--------------|
| Docker image | lloesche/valheim-server | wolveix/satisfactory-server |
| Ports | UDP 2456-2458 | UDP 7777 |
| Instance type | t3.medium (4GB) | t3.large (8GB) |
| Server password | Environment variable | In-game Server Manager |
| Data path | /opt/valheim/data | /config |

### Authentication

Satisfactory uses in-game Server Manager for security:
1. First player claims server and sets Admin Password
2. Admin sets Player Password for friends to join
3. Server is unlisted by default (not in public browser)

No terraform variables needed for passwords.

## File Structure

```
terraform/games/satisfactory/
├── backend.tf              # S3 state: bonfire/satisfactory/terraform.tfstate
├── main.tf                 # Game config + module calls
├── variables.tf            # Input variables
├── outputs.tf              # instance_id, public_ip, ssh_command
└── terraform.tfvars.example
```

## Deployment Flow

1. `cd terraform/games/satisfactory`
2. `terraform init`
3. `terraform apply`
4. Connect to server IP in-game, claim it, set passwords
5. Share player password with friends

## Backup & Restore

- Automatic S3 backup of `/config/saved` on instance stop
- Automatic restore from S3 on instance boot
- Bucket: `bonfire-satisfactory-backups-<region>`

## Discord Bot (Optional)

Enable with `enable_discord_bot = true` for:
- `/server satisfactory status`
- `/server satisfactory start`
- `/server satisfactory stop`

## References

- [wolveix/satisfactory-server](https://github.com/wolveix/satisfactory-server) - Docker image
- [Satisfactory Dedicated Servers Wiki](https://satisfactory.wiki.gg/wiki/Dedicated_servers) - Official docs
