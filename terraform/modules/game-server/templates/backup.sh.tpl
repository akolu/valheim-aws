#!/bin/bash
BACKUP_DIR="/tmp/${game_name}_backup"
S3_BUCKET="${s3_bucket}"
GAME_NAME="${game_name}"
BACKUP_RETENTION_COUNT=${backup_retention_count}

echo "Starting $GAME_NAME world backup..."

# Clean up any previous backup attempts
rm -rf $BACKUP_DIR
rm -f /tmp/$${GAME_NAME}_backup.tar.gz

# Create temp backup directory
mkdir -p $BACKUP_DIR

# Copy backup paths
%{ for path in backup_paths ~}
if [ -d "${path}" ]; then
  cp -r "${path}" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy ${path}"
elif [ -f "${path}" ]; then
  cp "${path}" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: Failed to copy ${path}"
else
  echo "Warning: ${path} not found"
fi
%{ endfor ~}

# Check if any files were copied
if [ ! "$(ls -A $BACKUP_DIR)" ]; then
  echo "Error: No files found to backup."
  echo "Backup failed but continuing shutdown process."
else
  # Create tarball
  tar -czf "/tmp/$${GAME_NAME}_backup.tar.gz" -C "/tmp" "$${GAME_NAME}_backup"

  # Generate timestamped key
  TIMESTAMP=$(date -u +"%Y-%m-%dT%H%M%SZ")
  TIMESTAMPED_KEY="$${GAME_NAME}_backup_$${TIMESTAMP}.tar.gz"
  LATEST_KEY="$${GAME_NAME}_backup_latest.tar.gz"

  # Upload timestamped backup
  if aws s3 cp "/tmp/$${GAME_NAME}_backup.tar.gz" "s3://$S3_BUCKET/$${TIMESTAMPED_KEY}"; then
    echo "Uploaded timestamped backup: $${TIMESTAMPED_KEY}"

    # Update _latest to point to the most recent backup
    if aws s3 cp "s3://$S3_BUCKET/$${TIMESTAMPED_KEY}" "s3://$S3_BUCKET/$${LATEST_KEY}"; then
      echo "Updated $${LATEST_KEY} to most recent backup"
    else
      echo "Warning: Failed to update $${LATEST_KEY}"
    fi

    # Prune old backups beyond retention count
    # List all timestamped backups sorted by date (oldest first), excluding _latest
    BACKUP_LIST=$(aws s3 ls "s3://$S3_BUCKET/" | grep "$${GAME_NAME}_backup_" | grep -v "_latest" | sort | awk '{print $NF}')
    BACKUP_COUNT=$(echo "$BACKUP_LIST" | grep -c '[^[:space:]]' || true)

    if [ "$BACKUP_COUNT" -gt "$BACKUP_RETENTION_COUNT" ]; then
      DELETE_COUNT=$(( BACKUP_COUNT - BACKUP_RETENTION_COUNT ))
      echo "Pruning $${DELETE_COUNT} old backup(s) (keeping $${BACKUP_RETENTION_COUNT})..."
      echo "$BACKUP_LIST" | head -n "$DELETE_COUNT" | while read -r OLD_KEY; do
        if aws s3 rm "s3://$S3_BUCKET/$${OLD_KEY}"; then
          echo "Deleted old backup: $${OLD_KEY}"
        else
          echo "Warning: Failed to delete $${OLD_KEY}"
        fi
      done
    fi
  else
    echo "Warning: Failed to upload backup to S3, but continuing shutdown process."
  fi
fi

# Cleanup local temp files
rm -rf $BACKUP_DIR /tmp/$${GAME_NAME}_backup.tar.gz
