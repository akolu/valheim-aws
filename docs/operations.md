# Operations Runbook

Day-to-day tasks for managing running bonfire game servers.

## Connecting to a Running Instance via SSM

No SSH keys or open ports required. Uses AWS Systems Manager Session Manager.

**Prerequisites:** Install the Session Manager plugin once:
```bash
# macOS
brew install --cask session-manager-plugin
```

**Get the instance ID:**
```bash
bonfire status <game>
# Instance ID:    i-0abc123def456789
```

**Start a shell session:**
```bash
aws ssm start-session --target <instance-id>
```

You'll land as `ssm-user`. Switch to root if needed: `sudo su -`

## Checking Server Logs

After connecting via SSM, logs are available via Docker Compose:

```bash
cd /opt/<game>
docker-compose logs -f
```

To view just the game container (excludes init containers):
```bash
docker-compose logs -f <game>-server
```

Use `Ctrl-C` to stop following. Drop `-f` to dump and exit.

## Where Save Files Live

Save data is stored on the host and bind-mounted into the container at the same path.

| Game | Path (host = container) |
|------|------------------------|
| Valheim | `/opt/valheim/data/worlds_local/` |
| Satisfactory | `/config/saved/` |
| Factorio | `/factorio/saves/` |

The full data directories are:
- Valheim: `/opt/valheim/data`
- Satisfactory: `/config`
- Factorio: `/factorio`

These directories exist on the host EC2 instance. You can inspect them directly after connecting via SSM.

## Checking Backup Status

**Bucket names:**
- Short-term: `bonfire-<game>-backups-<region>` (e.g. `bonfire-valheim-backups-eu-north-1`)
- Long-term: `<game>-long-term-backups` (e.g. `valheim-long-term-backups`)

**List recent backups:**
```bash
# Short-term (auto-backup bucket)
aws s3 ls s3://bonfire-<game>-backups-<region>/

# Long-term (retire archives)
aws s3 ls s3://<game>-long-term-backups/
```

**Check via CLI:**
```bash
bonfire status <game>
# Shows last backup object and long-term archive count
```

Backups are created automatically when the instance stops (spot interruption, `/stop` via Discord, or manual stop). The `_latest` key always points to the most recent backup.

## Starting and Stopping the Server Manually

The game runs as a systemd service. The service start/stop hooks run the restore and backup scripts automatically.

**Stop the server (triggers backup to S3):**
```bash
sudo systemctl stop <game>
```

**Start the server (triggers restore from S3):**
```bash
sudo systemctl start <game>
```

**Check service status:**
```bash
sudo systemctl status <game>
```

You can also control the container directly without triggering backup/restore:
```bash
cd /opt/<game>
docker-compose down   # stop
docker-compose up -d  # start
```

> **Note:** Using `docker-compose` directly bypasses the backup/restore scripts. Use `systemctl` for normal start/stop to ensure saves are preserved.
