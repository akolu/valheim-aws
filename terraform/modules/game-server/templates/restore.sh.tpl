#!/bin/bash
BACKUP_DIR="/tmp/${game_name}_backup"
S3_BUCKET="${s3_bucket}"
GAME_NAME="${game_name}"
DATA_PATH="${data_path}"

echo "Checking for existing $GAME_NAME data..."

# Create data directory if it doesn't exist
mkdir -p "$DATA_PATH"

# Check if data directory is empty or missing key files
if [ ! "$(ls -A $DATA_PATH 2>/dev/null)" ]; then
  echo "Data directory empty. Attempting to restore from backup..."

  # Clean up any previous restore attempts
  rm -rf $BACKUP_DIR
  rm -f /tmp/$${GAME_NAME}_backup.tar.gz

  # Download backup from S3
  if aws s3 cp "s3://$S3_BUCKET/$${GAME_NAME}_backup_latest.tar.gz" "/tmp/$${GAME_NAME}_backup.tar.gz"; then
    echo "Backup downloaded successfully"

    # Extract backup
    mkdir -p $BACKUP_DIR
    tar -xzf "/tmp/$${GAME_NAME}_backup.tar.gz" -C "/tmp"

    # Copy restored files to data path
    cp -r $BACKUP_DIR/* "$DATA_PATH/" 2>/dev/null || echo "Warning: Failed to copy restored files"

    # Set ownership (container typically runs as UID 1000)
    chown -R 1000:1000 "$DATA_PATH"

    echo "Data successfully restored from backup"

    # Cleanup
    rm -rf $BACKUP_DIR
    rm -f /tmp/$${GAME_NAME}_backup.tar.gz
  else
    echo "Warning: No backup found in S3 bucket $S3_BUCKET"
  fi
else
  echo "Existing data found. No restoration needed."
fi
