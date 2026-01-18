#!/bin/bash
BACKUP_DIR="/tmp/valheim_backup"
WORLD_DIR="/opt/valheim/data/worlds_local"
WORLD_NAME="${world_name}"
S3_BUCKET="${s3_bucket}"

echo "Starting Valheim world backup..."

# Clean up any previous backup attempts
rm -rf $BACKUP_DIR
rm -f /tmp/valheim_backup.tar.gz

# Create temp backup directory
mkdir -p $BACKUP_DIR

# Copy only the specific world files
cp "$WORLD_DIR/$WORLD_NAME.db" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: $WORLD_NAME.db not found"
cp "$WORLD_DIR/$WORLD_NAME.fwl" "$BACKUP_DIR/" 2>/dev/null || echo "Warning: $WORLD_NAME.fwl not found"

# Check if any files were copied
if [ ! "$(ls -A $BACKUP_DIR)" ]; then
  echo "Error: No world files found to backup. Check world name and directory."
  echo "Backup failed but continuing shutdown process."
else
  # Create tarball
  tar -czf "/tmp/valheim_backup.tar.gz" -C "/tmp" "valheim_backup"

  # Upload to S3, overwriting the existing backup
  if aws s3 cp "/tmp/valheim_backup.tar.gz" "s3://$S3_BUCKET/valheim_backup_latest.tar.gz"; then
    echo "Successfully updated backup in S3"
  else
    echo "Warning: Failed to upload backup to S3, but continuing shutdown process."
  fi
fi

# Cleanup local temp files
rm -rf $BACKUP_DIR /tmp/valheim_backup.tar.gz 