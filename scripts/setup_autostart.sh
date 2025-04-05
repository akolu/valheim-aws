#!/bin/bash
# Script to set up automatic startup for Valheim server on an existing instance
# Usage: ./setup_autostart.sh user@hostname key_file

if [ "$#" -lt 2 ]; then
    echo "Usage: $0 <user@hostname> <key_file>"
    echo "Example: $0 ec2-user@1.2.3.4 valheim-key.pem"
    exit 1
fi

SERVER="$1"
KEY_FILE="$2"

echo "Creating systemd service file for Valheim server auto-start..."
cat > /tmp/valheim.service << 'EOF'
[Unit]
Description=Valheim Server
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/valheim
ExecStart=/usr/local/bin/docker-compose up -d
ExecStop=/usr/local/bin/docker-compose down

[Install]
WantedBy=multi-user.target
EOF

echo "Copying service file to server..."
scp -i "$KEY_FILE" /tmp/valheim.service "$SERVER:/tmp/"

echo "Installing service on server..."
ssh -i "$KEY_FILE" "$SERVER" "sudo mv /tmp/valheim.service /etc/systemd/system/ && \
    sudo systemctl daemon-reload && \
    sudo systemctl enable valheim.service && \
    sudo systemctl start valheim.service"

if [ $? -eq 0 ]; then
    echo "✅ Valheim auto-start service has been installed and enabled"
    echo "The Valheim server will now automatically start when the instance boots"
else
    echo "❌ Failed to install auto-start service"
    exit 1
fi 