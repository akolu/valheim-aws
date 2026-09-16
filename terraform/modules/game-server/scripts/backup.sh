#!/bin/bash
# Archives the game's save data to S3 and prunes old history.
#
# Runs as ExecStop on the game's systemd unit, so it executes during shutdown.
# Configuration arrives as environment variables from that unit rather than
# being baked in at terraform render time, which keeps this file executable
# standalone and free of `$${}` escaping:
#
#   GAME_NAME               short game identifier, used in S3 key names
#   S3_BUCKET               short-term backup bucket
#   BACKUP_PATHS            space-separated paths to archive (paths may not contain spaces)
#   BACKUP_RETENTION_COUNT  number of timestamped backups to keep

# Fail loudly rather than backing up the wrong thing to the wrong place.
GAME_NAME="${GAME_NAME:?not set — expected from the systemd unit}"
S3_BUCKET="${S3_BUCKET:?not set — expected from the systemd unit}"
BACKUP_PATHS="${BACKUP_PATHS:?not set — expected from the systemd unit}"
BACKUP_RETENTION_COUNT="${BACKUP_RETENTION_COUNT:?not set — expected from the systemd unit}"

BACKUP_DIR="/tmp/${GAME_NAME}_backup"

echo "Starting $GAME_NAME world backup..."

# Clean up any previous backup attempts
rm -rf "$BACKUP_DIR"
rm -f "/tmp/${GAME_NAME}_backup.tar.gz"

# Create temp backup directory
mkdir -p "$BACKUP_DIR"

# Copy backup paths
read -r -a paths <<< "$BACKUP_PATHS"
for path in "${paths[@]}"; do
  if [ -d "$path" ]; then
    cp -r "$path" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy $path"
  elif [ -f "$path" ]; then
    cp "$path" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy $path"
  else
    echo "Warning: $path not found"
  fi
done

# Check if any files were copied
if [ ! "$(ls -A "$BACKUP_DIR")" ]; then
  echo "Error: No files found to backup."
  echo "Backup failed but continuing shutdown process."
else
  # Create tarball
  tar -czf "/tmp/${GAME_NAME}_backup.tar.gz" -C "/tmp" "${GAME_NAME}_backup"

  # Generate timestamped key
  TIMESTAMP=$(date -u +"%Y-%m-%dT%H%M%SZ")
  TIMESTAMPED_KEY="${GAME_NAME}_backup_${TIMESTAMP}.tar.gz"
  LATEST_KEY="${GAME_NAME}_backup_latest.tar.gz"

  # Upload _latest FIRST. It is the only key restore.sh reads, and this script
  # runs during shutdown — a spot reclaim allows roughly two minutes. Whichever
  # write goes second is the one lost when the box is cut off mid-shutdown, so
  # the pointer must not be the straggler. Observed 2026-09-11: the timestamped
  # object landed at 09:14:43Z and the instance died before the copy to _latest,
  # leaving the pointer ~12 hours stale while a good backup sat beside it.
  if aws s3 cp "/tmp/${GAME_NAME}_backup.tar.gz" "s3://$S3_BUCKET/${LATEST_KEY}"; then
    echo "Uploaded ${LATEST_KEY}"

    # Keep a timestamped copy for history (server-side copy, no re-upload)
    if aws s3 cp "s3://$S3_BUCKET/${LATEST_KEY}" "s3://$S3_BUCKET/${TIMESTAMPED_KEY}"; then
      echo "Uploaded timestamped backup: ${TIMESTAMPED_KEY}"
    else
      echo "Warning: Failed to write timestamped copy ${TIMESTAMPED_KEY}"
    fi

    # Prune old backups beyond retention count
    # List all timestamped backups sorted by date (oldest first), excluding _latest
    BACKUP_LIST=$(aws s3 ls "s3://$S3_BUCKET/" | grep "${GAME_NAME}_backup_" | grep -v "_latest" | sort | awk '{print $NF}')
    BACKUP_COUNT=$(echo "$BACKUP_LIST" | grep -c '[^[:space:]]' || true)

    if [ "$BACKUP_COUNT" -gt "$BACKUP_RETENTION_COUNT" ]; then
      DELETE_COUNT=$(( BACKUP_COUNT - BACKUP_RETENTION_COUNT ))
      echo "Pruning ${DELETE_COUNT} old backup(s) (keeping ${BACKUP_RETENTION_COUNT})..."
      echo "$BACKUP_LIST" | head -n "$DELETE_COUNT" | while read -r OLD_KEY; do
        if aws s3 rm "s3://$S3_BUCKET/${OLD_KEY}"; then
          echo "Deleted old backup: ${OLD_KEY}"
        else
          echo "Warning: Failed to delete ${OLD_KEY}"
        fi
      done
    fi
  else
    echo "Warning: Failed to upload backup to S3, but continuing shutdown process."
  fi
fi

# Cleanup local temp files
rm -rf "$BACKUP_DIR" "/tmp/${GAME_NAME}_backup.tar.gz"
