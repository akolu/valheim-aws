#!/bin/bash

INSTANCE_NAME="valheim-server"
REGION="eu-north-1"

print_usage() {
  echo "Usage: $0 [start|stop|status]"
  echo "  start  - Start the Valheim server"
  echo "  stop   - Stop the Valheim server"
  echo "  status - Check the status of the Valheim server"
}

if [ $# -ne 1 ]; then
  print_usage
  exit 1
fi

case "$1" in
  start)
    echo "Starting Valheim server..."
    aws lightsail start-instance --instance-name $INSTANCE_NAME --region $REGION
    ;;
  stop)
    echo "Stopping Valheim server..."
    aws lightsail stop-instance --instance-name $INSTANCE_NAME --region $REGION
    ;;
  status)
    echo "Checking Valheim server status..."
    aws lightsail get-instance --instance-name $INSTANCE_NAME --region $REGION --query 'instance.state.name' --output text
    ;;
  *)
    print_usage
    exit 1
    ;;
esac 