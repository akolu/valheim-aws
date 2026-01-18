#!/bin/bash
BACKUP_DIR="/tmp/${game_name}_backup"
S3_BUCKET="${s3_bucket}"
GAME_NAME="${game_name}"

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

  # Upload to S3, overwriting the existing backup
  if aws s3 cp "/tmp/$${GAME_NAME}_backup.tar.gz" "s3://$S3_BUCKET/$${GAME_NAME}_backup_latest.tar.gz"; then
    echo "Successfully updated backup in S3"
  else
    echo "Warning: Failed to upload backup to S3, but continuing shutdown process."
  fi
fi

# Cleanup local temp files
rm -rf $BACKUP_DIR /tmp/$${GAME_NAME}_backup.tar.gz
