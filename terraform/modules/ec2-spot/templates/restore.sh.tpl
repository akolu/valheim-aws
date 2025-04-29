#!/bin/bash
BACKUP_DIR="/tmp/valheim_backup"
WORLD_DIR="/opt/valheim/data/worlds_local"
WORLD_NAME="${world_name}"
S3_BUCKET="${s3_bucket}"

echo "Checking for existing Valheim world files..."

# Create world directory if it doesn't exist
mkdir -p "$WORLD_DIR"

# Check if world files exist
if [ ! -f "$WORLD_DIR/$WORLD_NAME.db" ] || [ ! -f "$WORLD_DIR/$WORLD_NAME.fwl" ]; then
  echo "World files missing. Attempting to restore from backup..."
  
  # Clean up any previous restore attempts
  rm -rf $BACKUP_DIR
  rm -f /tmp/valheim_backup.tar.gz
  
  # Download backup from S3
  if aws s3 cp "s3://$S3_BUCKET/valheim_backup_latest.tar.gz" "/tmp/valheim_backup.tar.gz"; then
    echo "Backup downloaded successfully"
    
    # Extract backup
    mkdir -p $BACKUP_DIR
    tar -xzf "/tmp/valheim_backup.tar.gz" -C "/tmp"
    
    # Move world files to the correct location
    cp "$BACKUP_DIR/$WORLD_NAME.db" "$WORLD_DIR/" 2>/dev/null || echo "Warning: $WORLD_NAME.db not found in backup"
    cp "$BACKUP_DIR/$WORLD_NAME.fwl" "$WORLD_DIR/" 2>/dev/null || echo "Warning: $WORLD_NAME.fwl not found in backup"
    
    # Check if restore was successful
    if [ -f "$WORLD_DIR/$WORLD_NAME.db" ] || [ -f "$WORLD_DIR/$WORLD_NAME.fwl" ]; then
      echo "World files successfully restored from backup"
    else
      echo "Warning: Could not restore world files from backup"
    fi
    
    # Cleanup
    rm -rf $BACKUP_DIR
    rm -f /tmp/valheim_backup.tar.gz
  else
    echo "Warning: No backup found in S3 bucket $S3_BUCKET"
  fi
else
  echo "Existing world files found. No restoration needed."
fi 